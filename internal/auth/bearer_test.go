package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"Bearer abc123", "abc123"},
		{"Bearer ", ""},
		{"Basic abc123", ""},
		{"", ""},
		{"bearer abc123", ""}, // case-sensitive
	}

	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		if tt.header != "" {
			r.Header.Set("Authorization", tt.header)
		}
		got := extractBearerToken(r)
		if got != tt.want {
			t.Errorf("extractBearerToken(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestBearerMiddlewareMissingToken(t *testing.T) {
	// Create a verifier with a dummy — we're testing the middleware logic, not JWKS
	v := &BearerVerifier{
		verifier:        nil,
		wwwAuthenticate: `Bearer`,
		wwwAuthError:    `Bearer error="invalid_token"`,
	}

	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("GET", "/mcp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Error("missing WWW-Authenticate header")
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	data, err := ProtectedResourceMetadata(
		"https://auth.civic-os.org/realms/central-os",
		"https://kb.civic-os.org",
	)
	if err != nil {
		t.Fatal(err)
	}

	s := string(data)
	if s == "" {
		t.Fatal("empty metadata")
	}
	// Basic structure checks
	if !contains(s, "resource") || !contains(s, "authorization_servers") {
		t.Errorf("metadata missing expected fields: %s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
