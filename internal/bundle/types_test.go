package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// testRegistry is a small registry for the lookup and validation tests.
func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry([]ConceptType{
		{Name: "Client Profile", Folder: "clients", Holds: "Clients"},
		{Name: "Competitive Analysis", Folder: "competitive-analysis", Holds: "Competitors"},
		{Name: "Infrastructure Component", Folder: "infrastructure", Holds: "Servers"},
		{Name: "Marketing Document", Folder: "marketing", Holds: "Brand and messaging"},
		{Name: "Strategy Document", Folder: "strategy", Holds: "Strategy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var deployDir = filepath.Join("..", "..", "deploy")

// configTypes returns the KB_TYPES value of a ConfigMap manifest.
func configTypes(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(deployDir, name))
	if err != nil {
		t.Fatal(err)
	}
	var cm struct {
		Data map[string]string `yaml:"data"`
	}
	if err := yaml.Unmarshal(data, &cm); err != nil {
		t.Fatalf("deploy/%s: %v", name, err)
	}
	raw, ok := cm.Data["KB_TYPES"]
	if !ok {
		t.Fatalf("deploy/%s has no KB_TYPES", name)
	}
	return raw
}

// instanceRegistry is the Civic OS type set: KB_TYPES from the committed
// deploy/configmap.yaml.example (the real configmap.yaml is gitignored).
func instanceRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := ParseRegistry([]byte(configTypes(t, "configmap.yaml.example")))
	if err != nil {
		t.Fatalf("deploy/configmap.yaml.example KB_TYPES: %v", err)
	}
	return r
}

func TestNewRegistryValidation(t *testing.T) {
	ok := ConceptType{Name: "Runbook", Folder: "runbooks", Holds: "Procedures"}
	tests := []struct {
		name        string
		types       []ConceptType
		errContains string
	}{
		{"empty", nil, "empty"},
		{"no name", []ConceptType{{Folder: "x", Holds: "x"}}, "has no name"},
		{"duplicate name", []ConceptType{ok, {Name: "runbook", Folder: "other", Holds: "x"}}, "duplicate name"},
		{"duplicate folder", []ConceptType{ok, {Name: "Other", Folder: "runbooks", Holds: "x"}}, `folder "runbooks" is already registered`},
		{"nested folder", []ConceptType{{Name: "A", Folder: "a/b", Holds: "x"}}, "not a valid top-level folder"},
		{"empty folder", []ConceptType{{Name: "A", Holds: "x"}}, "not a valid top-level folder"},
		{"dot folder", []ConceptType{{Name: "A", Folder: "..", Holds: "x"}}, "not a valid top-level folder"},
		{"no holds", []ConceptType{{Name: "A", Folder: "a"}}, "holds is empty"},
		{"bad color", []ConceptType{{Name: "A", Folder: "a", Holds: "x", Color: "red"}}, `color "red" is not #rrggbb`},
	}
	for _, tt := range tests {
		_, err := NewRegistry(tt.types)
		if err == nil || !strings.Contains(err.Error(), tt.errContains) {
			t.Errorf("%s: err = %v, want containing %q", tt.name, err, tt.errContains)
		}
	}

	r, err := NewRegistry([]ConceptType{{Name: " Runbook ", Folder: " runbooks ", Holds: "Procedures", Color: "#10B981"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := r.ForFolder("runbooks"); got.Name != "Runbook" {
		t.Errorf("names and folders should be trimmed: %+v", got)
	}
}

func TestParseRegistry(t *testing.T) {
	if _, err := ParseRegistry([]byte(`[{"name":"A","folder":"a","holds":"x","colour":"#000000"}]`)); err == nil || !strings.Contains(err.Error(), "colour") {
		t.Errorf("unknown field should be rejected: %v", err)
	}
	if _, err := ParseRegistry([]byte(`{"name":"A"}`)); err == nil {
		t.Error("a non-array should be rejected")
	}
	if _, err := ParseRegistry([]byte(`[{"name":"A","folder":"a","holds":"x"}] []`)); err == nil {
		t.Error("trailing data should be rejected")
	}
	r, err := ParseRegistry([]byte(`[{"name":"A","folder":"a","holds":"x"}]`))
	if err != nil || len(r.Types()) != 1 {
		t.Errorf("ParseRegistry = %v, %v", r, err)
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	for _, folder := range []string{"decisions", "meeting-notes", "projects", "research", "runbooks", "tools"} {
		if _, ok := r.ForFolder(folder); !ok {
			t.Errorf("defaults should include %s/", folder)
		}
	}
	if _, ok := r.ForFolder("clients"); ok {
		t.Error("defaults should be generic; clients/ belongs in a deployment's KB_TYPES")
	}
}

// TestExampleConfigTypes checks the committed Civic OS KB_TYPES, so a broken or
// incomplete type set fails here rather than at pod start.
func TestExampleConfigTypes(t *testing.T) {
	r := instanceRegistry(t)
	want := map[string]string{
		"clients": "Client Profile", "company": "Company Reference",
		"competitive-analysis": "Competitive Analysis", "decisions": "Decision Record",
		"infrastructure": "Infrastructure Component", "instances": "Instance Deployment",
		"marketing": "Marketing Document", "meeting-notes": "Meeting Note",
		"partners": "Partner Profile", "module": "Project Module",
		"projects": "Project Specification", "proposals": "Proposal",
		"prospects": "Prospect", "research": "Research Analysis",
		"runbooks": "Runbook", "strategy": "Strategy Document", "tools": "Tool Reference",
	}
	for folder, name := range want {
		if got, ok := r.ForFolder(folder); !ok || got.Name != name {
			t.Errorf("KB_TYPES: %s/ = %q, want %q", folder, got.Name, name)
		}
	}
	if n := len(r.Types()); n != len(want) {
		t.Errorf("KB_TYPES has %d types, want %d (update this test when adding one)", n, len(want))
	}
}

// TestLocalConfigTypesMatchExample checks the real deploy/configmap.yaml, which
// is gitignored, when it exists: its KB_TYPES must parse and match the example,
// so the committed type set (and the tests built on it) can't drift from what
// is deployed. CI has no local file and skips.
func TestLocalConfigTypesMatchExample(t *testing.T) {
	if _, err := os.Stat(filepath.Join(deployDir, "configmap.yaml")); errors.Is(err, os.ErrNotExist) {
		t.Skip("no local deploy/configmap.yaml")
	}
	local, err := ParseRegistry([]byte(configTypes(t, "configmap.yaml")))
	if err != nil {
		t.Fatalf("deploy/configmap.yaml KB_TYPES: %v", err)
	}
	got, want := local.Types(), instanceRegistry(t).Types()
	if reflect.DeepEqual(got, want) {
		return
	}
	var diffs []string
	for i := 0; i < len(got) || i < len(want); i++ {
		var g, w ConceptType
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g != w {
			diffs = append(diffs, fmt.Sprintf("  entry %d: local %+v, example %+v", i+1, g, w))
		}
	}
	t.Errorf("deploy/configmap.yaml KB_TYPES differs from configmap.yaml.example; keep them in step:\n%s", strings.Join(diffs, "\n"))
}

func TestResolveType(t *testing.T) {
	r := testRegistry(t)
	tests := []struct {
		path, requested, want, errContains string
	}{
		{"clients/neh.md", "", "Client Profile", ""},
		{"clients/archive/old.md", "", "Client Profile", ""},
		{"marketing/voice.md", "marketing document", "Marketing Document", ""},
		{"marketing/voice.md", " Marketing Document ", "Marketing Document", ""},
		{"strategy/voice.md", "Marketing Document", "", "Marketing Document concepts live in marketing/ (e.g. marketing/voice.md)"},
		{"strategy/pricing.md", "Pricing Model", "", `unknown type "Pricing Model"; strategy/ holds Strategy Document`},
		{"competitors/vendor.md", "Competitive Analysis", "", "not in a registered folder; concepts live in clients/, competitive-analysis/"},
		{"a.md", "", "", "not in a registered folder"},
	}
	for _, tt := range tests {
		got, err := r.Resolve(tt.path, tt.requested)
		if tt.errContains != "" {
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Resolve(%q, %q) err = %v, want containing %q", tt.path, tt.requested, err, tt.errContains)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("Resolve(%q, %q) = %q, %v; want %q", tt.path, tt.requested, got, err, tt.want)
		}
	}
}

func TestTypeProblem(t *testing.T) {
	r := testRegistry(t)
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
		if got := r.Problem(c); got != tt.want {
			t.Errorf("Problem(%s) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestTemplatesMatchRegistry keeps the example templates in step with our
// instance's types: every template uses a configured type, and the bundle
// index lists every configured folder.
func TestTemplatesMatchRegistry(t *testing.T) {
	r := instanceRegistry(t)
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
		if _, ok := r.ByName(c.Meta.Type); !ok {
			t.Errorf("template %s uses unconfigured type %q", e.Name(), c.Meta.Type)
		}
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ct := range r.Types() {
		if !strings.Contains(string(index), "("+ct.Folder+"/)") {
			t.Errorf("templates/index.md doesn't list %s/", ct.Folder)
		}
	}
}
