package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
)

func TestRequireAuthRedirectsUnauthenticated(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!"))
	auth := &OIDCAuth{
		store:        store,
		allowedRoles: map[string]bool{"kb-viewer": true},
	}

	handler := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("protected content"))
	}))

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/auth/login" {
		t.Errorf("redirect location = %q, want /auth/login", loc)
	}
}

func TestRequireAuthAllowsAuthenticated(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!"))
	auth := &OIDCAuth{
		store:        store,
		allowedRoles: map[string]bool{"kb-viewer": true},
	}

	handler := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("protected content"))
	}))

	// Simulate an authenticated session
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	session, _ := store.Get(r, "kb-session")
	session.Values["authenticated"] = true
	session.Save(r, w)

	// Copy cookies to new request
	r2 := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range w.Result().Cookies() {
		r2.AddCookie(cookie)
	}
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w2.Code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-key-32-bytes-long!!"))
	auth := &OIDCAuth{
		store: store,
	}

	handler := auth.LogoutHandler()

	r := httptest.NewRequest("GET", "/auth/logout", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
}

func TestRandomState(t *testing.T) {
	s1 := randomState()
	s2 := randomState()

	if len(s1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("state length = %d, want 32", len(s1))
	}
	if s1 == s2 {
		t.Error("random states should be different")
	}
}

func TestKeycloakClaimsRoles(t *testing.T) {
	raw := `{
		"preferred_username": "dan",
		"realm_access": {"roles": ["default-roles", "kb-admin"]},
		"resource_access": {
			"civic-os-kb": {"roles": ["kb-viewer"]},
			"account": {"roles": ["manage-account"]}
		}
	}`
	var claims keycloakClaims
	if err := json.Unmarshal([]byte(raw), &claims); err != nil {
		t.Fatal(err)
	}
	got := claims.roles("civic-os-kb")
	want := []string{"default-roles", "kb-admin", "kb-viewer"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("roles = %v, want %v (other clients' roles excluded)", got, want)
	}
	if got := (keycloakClaims{}).roles("civic-os-kb"); len(got) != 0 {
		t.Errorf("empty claims should have no roles, got %v", got)
	}
}

func TestHasAllowedRole(t *testing.T) {
	a := &OIDCAuth{allowedRoles: map[string]bool{"kb-viewer": true, "kb-admin": true}}
	if !a.hasAllowedRole([]string{"offline_access", "kb-admin"}) {
		t.Error("kb-admin should be allowed")
	}
	if a.hasAllowedRole([]string{"offline_access"}) || a.hasAllowedRole(nil) {
		t.Error("roles without kb-viewer/kb-admin should be denied")
	}
	if !(&OIDCAuth{}).hasAllowedRole(nil) {
		t.Error("no configured roles should allow everyone")
	}
	if got := a.allowedRoleList(); strings.Join(got, ",") != "kb-admin,kb-viewer" {
		t.Errorf("allowedRoleList = %v", got)
	}
}

func TestAccessTokenRoles(t *testing.T) {
	const issuer = "https://auth.example.test/realms/central-os"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(claims map[string]any) string {
		t.Helper()
		raw, err := josejwt.Signed(signer).Claims(claims).Serialize()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	token := func(azp string) map[string]any {
		return map[string]any{
			"iss": issuer, "aud": "account", "azp": azp, "sub": "u1",
			"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
			"realm_access":    map[string]any{"roles": []string{"kb-admin"}},
			"resource_access": map[string]any{"civic-os-kb": map[string]any{"roles": []string{"kb-viewer"}}},
		}
	}

	a := &OIDCAuth{
		oauth2Config: &oauth2.Config{ClientID: "civic-os-kb"},
		accessVerifier: oidc.NewVerifier(issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}},
			&oidc.Config{SkipClientIDCheck: true, SupportedSigningAlgs: []string{oidc.RS256}}),
	}
	ctx := context.Background()

	roles, err := a.accessTokenRoles(ctx, sign(token("civic-os-kb")))
	if err != nil || strings.Join(roles, ",") != "kb-admin,kb-viewer" {
		t.Errorf("roles = %v, err = %v", roles, err)
	}

	if _, err := a.accessTokenRoles(ctx, sign(token("some-other-client"))); err == nil {
		t.Error("a token issued to another client must be rejected")
	}

	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherSigner, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: other}, nil)
	forged, _ := josejwt.Signed(otherSigner).Claims(token("civic-os-kb")).Serialize()
	if _, err := a.accessTokenRoles(ctx, forged); err == nil {
		t.Error("a token with a bad signature must be rejected")
	}

	if _, err := a.accessTokenRoles(ctx, ""); err == nil {
		t.Error("a missing access token must be an error")
	}
}
