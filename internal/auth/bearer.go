package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// BearerConfig configures JWT Bearer token validation.
type BearerConfig struct {
	IssuerURL   string // Keycloak realm URL, e.g. https://auth.civic-os.org/realms/central-os
	ClientID    string // Expected audience
	InternalURL string // Optional: internal Keycloak base URL for Docker environments
	ResourceURL string // Public URL of this server (for resource_metadata in WWW-Authenticate)
}

// BearerVerifier validates JWT Bearer tokens against a Keycloak JWKS endpoint.
type BearerVerifier struct {
	verifier       *oidc.IDTokenVerifier
	wwwAuthenticate string // precomputed WWW-Authenticate header value
	wwwAuthError    string // precomputed WWW-Authenticate with error
}

// NewBearerVerifier creates a verifier that checks tokens against the OIDC provider's JWKS.
func NewBearerVerifier(ctx context.Context, cfg BearerConfig) (*BearerVerifier, error) {
	ctx, _, err := contextWithProxyClient(ctx, cfg.IssuerURL, cfg.InternalURL)
	if err != nil {
		return nil, fmt.Errorf("init proxy client: %w", err)
	}

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("init OIDC provider: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	wwwAuth := `Bearer`
	wwwAuthErr := `Bearer error="invalid_token"`
	if cfg.ResourceURL != "" {
		metadataURL := strings.TrimRight(cfg.ResourceURL, "/") + "/.well-known/oauth-protected-resource"
		wwwAuth = fmt.Sprintf(`Bearer resource_metadata=%q`, metadataURL)
		wwwAuthErr = fmt.Sprintf(`Bearer error="invalid_token", resource_metadata=%q`, metadataURL)
	}

	return &BearerVerifier{
		verifier:        verifier,
		wwwAuthenticate: wwwAuth,
		wwwAuthError:    wwwAuthErr,
	}, nil
}

// Middleware returns HTTP middleware that requires a valid Bearer token.
func (b *BearerVerifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			w.Header().Set("WWW-Authenticate", b.wwwAuthenticate)
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		_, err := b.verifier.Verify(r.Context(), token)
		if err != nil {
			w.Header().Set("WWW-Authenticate", b.wwwAuthError)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
