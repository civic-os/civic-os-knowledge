package viz

import (
	"strings"
	"testing"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
)

func TestGenerateEmpty(t *testing.T) {
	html, err := Generate(nil)
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

	html, err := Generate(concepts)
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

	html, err := Generate(concepts)
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

	html, err := Generate(concepts)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(html, `"edges":[{`) {
		t.Error("should not have self-link edges")
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		source, target, want string
	}{
		{"clients/a.md", "/instances/b.md", "instances/b.md"},
		{"clients/a.md", "b.md", "clients/b.md"},
		{"a.md", "b.md", "b.md"},
		{"deep/nested/a.md", "/top.md", "top.md"},
	}

	for _, tt := range tests {
		got := normalizePath(tt.source, tt.target)
		if got != tt.want {
			t.Errorf("normalizePath(%q, %q) = %q, want %q", tt.source, tt.target, got, tt.want)
		}
	}
}
