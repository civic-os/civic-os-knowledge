package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
)

// OIDCConfig configures the browser-based OIDC authentication flow.
type OIDCConfig struct {
	IssuerURL   string   // Keycloak realm URL (external, for browser redirects)
	InternalURL string   // Optional: internal Keycloak base URL for Docker environments
	ClientID     string   // OIDC client ID
	ClientSecret string   // OIDC client secret
	ExternalURL  string   // Public URL of this service (for redirect URI)
	SessionKey   string   // Secret key for cookie encryption
	AllowedRoles []string // Keycloak realm roles that grant access
}

// OIDCAuth handles browser-based OIDC authentication with Keycloak.
type OIDCAuth struct {
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
	store        sessions.Store
	allowedRoles map[string]bool
	httpClient   *http.Client // custom HTTP client for proxied environments
	endSessionURL string      // Keycloak end_session_endpoint
	externalURL   string      // this service's public URL (for post-logout redirect)
}

// NewOIDCAuth initializes the OIDC auth handler.
func NewOIDCAuth(ctx context.Context, cfg OIDCConfig) (*OIDCAuth, error) {
	ctx, httpClient, err := contextWithProxyClient(ctx, cfg.IssuerURL, cfg.InternalURL)
	if err != nil {
		return nil, fmt.Errorf("init proxy client: %w", err)
	}

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("init OIDC provider: %w", err)
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  strings.TrimRight(cfg.ExternalURL, "/") + "/auth/callback",
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "roles"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	// Extract end_session_endpoint from OIDC discovery
	var providerClaims struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	provider.Claims(&providerClaims)

	store := sessions.NewCookieStore([]byte(cfg.SessionKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		Secure:   strings.HasPrefix(cfg.ExternalURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	}

	roles := make(map[string]bool, len(cfg.AllowedRoles))
	for _, r := range cfg.AllowedRoles {
		roles[r] = true
	}

	return &OIDCAuth{
		oauth2Config:  oauth2Config,
		verifier:      verifier,
		store:         store,
		allowedRoles:  roles,
		httpClient:    httpClient,
		endSessionURL: providerClaims.EndSessionEndpoint,
		externalURL:   cfg.ExternalURL,
	}, nil
}

// LoginHandler initiates the OIDC authorization code flow.
func (a *OIDCAuth) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := randomState()
		session, _ := a.store.Get(r, "kb-session")
		session.Values["oauth_state"] = state
		session.Save(r, w)

		http.Redirect(w, r, a.oauth2Config.AuthCodeURL(state), http.StatusFound)
	}
}

// CallbackHandler handles the OIDC redirect callback.
func (a *OIDCAuth) CallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := a.store.Get(r, "kb-session")

		// Verify state
		expectedState, _ := session.Values["oauth_state"].(string)
		if r.URL.Query().Get("state") != expectedState {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}

		// Exchange code for token (use proxy client if configured)
		exchangeCtx := r.Context()
		if a.httpClient != nil {
			exchangeCtx = context.WithValue(exchangeCtx, oauth2.HTTPClient, a.httpClient)
		}
		token, err := a.oauth2Config.Exchange(exchangeCtx, r.URL.Query().Get("code"))
		if err != nil {
			log.Printf("OIDC exchange error: %v", err)
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}

		// Extract ID token
		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok {
			http.Error(w, "missing id_token", http.StatusInternalServerError)
			return
		}

		idToken, err := a.verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			http.Error(w, "invalid id_token", http.StatusUnauthorized)
			return
		}

		// Check roles
		if !a.hasRequiredRole(idToken) {
			http.Error(w, "access denied: insufficient role", http.StatusForbidden)
			return
		}

		session.Values["authenticated"] = true
		delete(session.Values, "oauth_state")
		session.Save(r, w)

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// LogoutHandler clears the session and redirects to Keycloak logout.
func (a *OIDCAuth) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := a.store.Get(r, "kb-session")
		opts := *session.Options
		opts.MaxAge = -1
		session.Options = &opts
		session.Values["authenticated"] = false
		session.Save(r, w)

		// Redirect to Keycloak end_session_endpoint to clear SSO session
		if a.endSessionURL != "" {
			logoutURL := a.endSessionURL +
				"?client_id=" + a.oauth2Config.ClientID +
				"&post_logout_redirect_uri=" + a.externalURL
			http.Redirect(w, r, logoutURL, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// RequireAuth is middleware that checks for a valid OIDC session.
func (a *OIDCAuth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, _ := a.store.Get(r, "kb-session")
		authed, _ := session.Values["authenticated"].(bool)
		if !authed {
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *OIDCAuth) hasRequiredRole(token *oidc.IDToken) bool {
	if len(a.allowedRoles) == 0 {
		return true
	}

	var claims struct {
		RealmAccess struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if err := token.Claims(&claims); err != nil {
		return false
	}

	for _, role := range claims.RealmAccess.Roles {
		if a.allowedRoles[role] {
			return true
		}
	}
	return false
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ProtectedResourceMetadata returns the OAuth 2.0 Protected Resource Metadata JSON.
func ProtectedResourceMetadata(issuerURL, resourceURL string) ([]byte, error) {
	metadata := map[string]interface{}{
		"resource":                resourceURL,
		"authorization_servers":  []string{issuerURL},
		"bearer_methods_supported": []string{"header"},
	}
	return json.Marshal(metadata)
}
