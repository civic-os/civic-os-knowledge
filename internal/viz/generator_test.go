package viz

import (
	"strings"
	"testing"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
)

func TestGenerateEmpty(t *testing.T) {
	html, err := Generate(nil, bundle.DefaultRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Civic OS Knowledge Graph") {
		t.Error("missing title")
	}
	// With no concepts, nodes should be null or empty
	if strings.Contains(html, `"nodes":[{`) {
		t.Error("expected no real nodes")
	}
}

func TestGenerateWithNodes(t *testing.T) {
	concepts := []*bundle.Concept{
		{
			Meta: bundle.ConceptMeta{
				Type:        "Client Profile",
				Title:       "Mott Park",
				Description: "Recreation center.",
				Tags:        []string{"customer"},
			},
			Body: "# Overview\n\nSee [deployment runbook](/instances/mottpark-deployment.md).",
			Path: "clients/mottpark.md",
		},
		{
			Meta: bundle.ConceptMeta{
				Type:  "Instance Deployment",
				Title: "Mott Park Deployment",
			},
			Body: "Deploy steps.",
			Path: "instances/mottpark-deployment.md",
		},
	}

	html, err := Generate(concepts, bundle.DefaultRegistry())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(html, "Mott Park") {
		t.Error("missing node title")
	}
	if !strings.Contains(html, "clients/mottpark.md") {
		t.Error("missing node ID")
	}
	if !strings.Contains(html, "instances/mottpark-deployment.md") {
		t.Error("missing edge target")
	}
}

func TestGenerateEdgeDetection(t *testing.T) {
	concepts := []*bundle.Concept{
		{
			Meta: bundle.ConceptMeta{Type: "Note", Title: "A"},
			Body: "Links to [B](b.md) and [external](https://example.com).",
			Path: "a.md",
		},
		{
			Meta: bundle.ConceptMeta{Type: "Note", Title: "B"},
			Body: "Links back to [A](a.md).",
			Path: "b.md",
		},
	}

	html, err := Generate(concepts, bundle.DefaultRegistry())
	if err != nil {
		t.Fatal(err)
	}

	// Should have edges for the cross-links
	if !strings.Contains(html, `"source":"a.md"`) {
		t.Error("missing edge from a.md to b.md")
	}
	if !strings.Contains(html, `"source":"b.md"`) {
		t.Error("missing edge from b.md to a.md")
	}
}

func TestGenerateNoSelfLinks(t *testing.T) {
	concepts := []*bundle.Concept{
		{
			Meta: bundle.ConceptMeta{Type: "Note", Title: "Self"},
			Body: "Links to [self](self.md).",
			Path: "self.md",
		},
	}

	html, err := Generate(concepts, bundle.DefaultRegistry())
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(html, `"edges":[{`) {
		t.Error("should not have self-link edges")
	}
}

func TestGenerateDuplicateEdges(t *testing.T) {
	concepts := []*bundle.Concept{
		{
			Meta: bundle.ConceptMeta{Type: "Note", Title: "A"},
			Body: "Links to [B](b.md) and again [B](b.md).",
			Path: "a.md",
		},
		{
			Meta: bundle.ConceptMeta{Type: "Note", Title: "B"},
			Body: "No links.",
			Path: "b.md",
		},
	}

	html, err := Generate(concepts, bundle.DefaultRegistry())
	if err != nil {
		t.Fatal(err)
	}

	count := strings.Count(html, `"source":"a.md","target":"b.md"`)
	if count != 1 {
		t.Errorf("expected 1 edge from a.md to b.md, got %d", count)
	}
}

func TestTypeColors(t *testing.T) {
	types, err := bundle.NewRegistry([]bundle.ConceptType{
		{Name: "Project Module", Folder: "module", Holds: "Modules", Color: "#93c5fd"},
		{Name: "Uncolored", Folder: "uncolored", Holds: "No color set"},
	})
	if err != nil {
		t.Fatal(err)
	}
	colors := typeColors(types)
	if colors["Project Module"] != "#93c5fd" {
		t.Errorf("configured color not used: %q", colors["Project Module"])
	}
	fallback := colors["Uncolored"]
	if fallback == "" || fallback == "#6b7280" {
		t.Errorf("a type without a color should get a palette color, got %q", fallback)
	}

	// The fallback depends only on the name, not on registry order.
	reordered, _ := bundle.NewRegistry([]bundle.ConceptType{
		{Name: "Other", Folder: "other", Holds: "x"},
		{Name: "Uncolored", Folder: "uncolored", Holds: "No color set"},
	})
	if got := typeColors(reordered)["Uncolored"]; got != fallback {
		t.Errorf("fallback color moved with registry order: %q vs %q", got, fallback)
	}

	html, err := Generate(nil, types)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "/*TYPE_COLORS*/") || !strings.Contains(html, `"Project Module":"#93c5fd"`) {
		t.Error("type colors not injected into the template")
	}
}
