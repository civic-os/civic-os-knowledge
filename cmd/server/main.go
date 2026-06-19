package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/civic-os/civic-os-knowledge/internal/auth"
	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	mcpserver "github.com/civic-os/civic-os-knowledge/internal/mcp"
	"github.com/civic-os/civic-os-knowledge/internal/search"
	"github.com/civic-os/civic-os-knowledge/internal/storage"
	"github.com/civic-os/civic-os-knowledge/internal/tools"
	"github.com/civic-os/civic-os-knowledge/internal/viz"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	bundleDir := envOr("KB_BUNDLE_DIR", "/data/knowledge/bundle")
	port := envOr("KB_PORT", "8080")

	b, err := bundle.NewBundle(bundleDir)
	if err != nil {
		log.Fatalf("init bundle: %v", err)
	}

	// S3 sync (optional)
	var syncer *storage.Syncer
	if s3Endpoint := os.Getenv("KB_S3_ENDPOINT"); s3Endpoint != "" {
		ctx := context.Background()
		store, err := storage.NewSpacesStore(ctx, storage.SpacesConfig{
			Endpoint:  s3Endpoint,
			Region:    envOr("KB_S3_REGION", "nyc3"),
			Bucket:    os.Getenv("KB_S3_BUCKET"),
			Prefix:    envOr("KB_S3_PREFIX", "knowledge/"),
			AccessKey: os.Getenv("KB_S3_ACCESS_KEY"),
			SecretKey: os.Getenv("KB_S3_SECRET_KEY"),
			PathStyle: os.Getenv("KB_S3_PATH_STYLE") == "true",
		})
		if err != nil {
			log.Fatalf("init S3: %v", err)
		}

		syncer = storage.NewSyncer(store, bundleDir)
		if err := syncer.Pull(ctx); err != nil {
			log.Printf("WARNING: S3 pull failed: %v", err)
		}
	}

	// Build search index
	idx := search.NewIndex()
	concepts, err := b.List()
	if err != nil {
		log.Fatalf("list bundle: %v", err)
	}
	idx.BuildFromBundle(concepts)
	log.Printf("indexed %d concepts from %s", len(concepts), bundleDir)

	// viz.html directory (sibling of bundle dir)
	vizDir := filepath.Dir(bundleDir)

	// Generate initial viz.html
	regenViz := func() {
		concepts, err := b.List()
		if err != nil {
			log.Printf("WARNING: viz regen list: %v", err)
			return
		}
		html, err := viz.Generate(concepts)
		if err != nil {
			log.Printf("WARNING: viz regen generate: %v", err)
			return
		}
		if err := os.WriteFile(filepath.Join(vizDir, "viz.html"), []byte(html), 0o644); err != nil {
			log.Printf("WARNING: viz regen write: %v", err)
			return
		}
		log.Printf("regenerated viz.html (%d concepts)", len(concepts))
	}
	regenViz()

	deps := &tools.Deps{
		Bundle: b,
		Index:  idx,
		OnWrite: func(path string) {
			log.Printf("concept written: %s", path)
			if syncer != nil {
				syncer.PushFile(path)
			}
			regenViz()
		},
	}

	server := mcpserver.NewMCPServer(deps)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
		},
	)

	mux := http.NewServeMux()

	// Health endpoint (always public)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	// OAuth Protected Resource Metadata (always public)
	keycloakURL := os.Getenv("KB_KEYCLOAK_URL")
	keycloakRealm := os.Getenv("KB_KEYCLOAK_REALM")
	keycloakInternalURL := os.Getenv("KB_KEYCLOAK_INTERNAL_URL")
	externalURL := os.Getenv("KB_EXTERNAL_URL")
	if keycloakURL != "" && keycloakRealm != "" {
		issuerURL := strings.TrimRight(keycloakURL, "/") + "/realms/" + keycloakRealm
		internalIssuerURL := ""
		if keycloakInternalURL != "" {
			internalIssuerURL = strings.TrimRight(keycloakInternalURL, "/") + "/realms/" + keycloakRealm
		}
		metadata, _ := auth.ProtectedResourceMetadata(issuerURL, externalURL)
		mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(metadata)
		})

		// Bearer auth for MCP endpoint
		ctx := context.Background()
		bearer, err := auth.NewBearerVerifier(ctx, auth.BearerConfig{
			IssuerURL:   issuerURL,
			ClientID:    envOr("KB_KEYCLOAK_CLIENT_ID", "civic-os-kb"),
			InternalURL: internalIssuerURL,
			ResourceURL: externalURL,
		})
		if err != nil {
			log.Fatalf("init bearer auth: %v", err)
		}
		mux.Handle("/mcp", bearer.Middleware(mcpHandler))

		// OIDC browser auth for viz.html
		oidcAuth, err := auth.NewOIDCAuth(ctx, auth.OIDCConfig{
			IssuerURL:    issuerURL,
			InternalURL:  internalIssuerURL,
			ClientID:     envOr("KB_KEYCLOAK_CLIENT_ID", "civic-os-kb"),
			ClientSecret: os.Getenv("KB_KEYCLOAK_CLIENT_SECRET"),
			ExternalURL:  externalURL,
			SessionKey:   envOr("KB_SESSION_KEY", "change-me-in-production-32bytes"),
			AllowedRoles: strings.Split(envOr("KB_VIZ_ROLES", "kb-viewer,kb-admin"), ","),
		})
		if err != nil {
			log.Fatalf("init OIDC auth: %v", err)
		}
		mux.HandleFunc("/auth/login", oidcAuth.LoginHandler())
		mux.HandleFunc("/auth/callback", oidcAuth.CallbackHandler())
		mux.HandleFunc("/auth/logout", oidcAuth.LogoutHandler())
		mux.Handle("/", oidcAuth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(vizDir, "viz.html"))
		})))
	} else {
		// No auth configured — serve everything without auth (development mode)
		log.Println("WARNING: no Keycloak configured, running without authentication")
		mux.Handle("/mcp", mcpHandler)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(vizDir, "viz.html"))
		})
	}

	// Graceful shutdown
	httpServer := &http.Server{Addr: ":" + port, Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		log.Println("shutting down...")
		if syncer != nil {
			syncer.Close()
		}
		httpServer.Shutdown(context.Background())
	}()

	log.Printf("listening on :%s", port)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
