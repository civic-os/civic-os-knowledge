package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/sessions"
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
