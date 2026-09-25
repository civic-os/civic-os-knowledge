package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypeRegistryIntegrity(t *testing.T) {
	names := make(map[string]bool)
	folders := make(map[string]bool)
	for _, ct := range Types {
		if names[strings.ToLower(ct.Name)] || folders[ct.Folder] {
			t.Errorf("duplicate registry entry: %+v", ct)
		}
		names[strings.ToLower(ct.Name)] = true
		folders[ct.Folder] = true
		if err := ValidatePath(ct.Folder + "/x.md"); err != nil || strings.Contains(ct.Folder, "/") {
			t.Errorf("folder %q is not a valid top-level folder", ct.Folder)
		}
		if ct.Holds == "" {
			t.Errorf("%s has no description", ct.Name)
		}
	}
}

func TestResolveType(t *testing.T) {
	tests := []struct {
		path, requested, want, errContains string
	}{
		{"clients/neh.md", "", "Client Profile", ""},
		{"clients/archive/old.md", "", "Client Profile", ""},
		{"marketing/voice.md", "marketing document", "Marketing Document", ""},
		{"marketing/voice.md", " Marketing Document ", "Marketing Document", ""},
		{"strategy/voice.md", "Marketing Document", "", "Marketing Document concepts live in marketing/ (e.g. marketing/voice.md)"},
		{"strategy/pricing.md", "Pricing Model", "", `unknown type "Pricing Model"; strategy/ holds Strategy Document`},
		{"competitors/vendor.md", "Competitive Analysis", "", "not in a registered folder"},
		{"a.md", "", "", "not in a registered folder"},
	}
	for _, tt := range tests {
		got, err := ResolveType(tt.path, tt.requested)
		if tt.errContains != "" {
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("ResolveType(%q, %q) err = %v, want containing %q", tt.path, tt.requested, err, tt.errContains)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ResolveType(%q, %q) = %q, %v; want %q", tt.path, tt.requested, got, err, tt.want)
		}
	}
}

func TestTypeProblem(t *testing.T) {
	tests := []struct {
		path, typ, want string
	}{
		{"clients/a.md", "Client Profile", ""},
		{"infrastructure/og.md", "Infrastructure", `infrastructure/og.md has type "Infrastructure" but infrastructure/ holds Infrastructure Component concepts`},
		{"competitors/vendor.md", "Competitive Analysis", "competitors/vendor.md is outside the registered folders; move it to competitive-analysis/vendor.md"},
		{"product/features.md", "Feature List", "product/features.md is outside the registered folders"},
	}
	for _, tt := range tests {
		c := &Concept{Path: tt.path, Meta: ConceptMeta{Type: tt.typ}}
		if got := TypeProblem(c); got != tt.want {
			t.Errorf("TypeProblem(%s) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestTemplatesMatchRegistry keeps the example templates in step with the
// registry: every template uses a registered type, and the bundle index
// lists every registered folder.
func TestTemplatesMatchRegistry(t *testing.T) {
	dir := filepath.Join("..", "..", "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "index.md" || e.Name() == "log.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		c, err := ParseConcept(data, e.Name())
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		if _, ok := TypeByName(c.Meta.Type); !ok {
			t.Errorf("template %s uses unregistered type %q", e.Name(), c.Meta.Type)
		}
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ct := range Types {
		if !strings.Contains(string(index), "("+ct.Folder+"/)") {
			t.Errorf("templates/index.md doesn't list %s/", ct.Folder)
		}
	}
}
