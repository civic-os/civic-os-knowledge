package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConcept(t *testing.T) {
	data := []byte(`---
type: Client Profile
title: Mott Park Recreation
description: Clubhouse reservation system with payment tracking.
resource: https://mottpark.civic-os.org
tags: [customer, payments, production]
timestamp: 2026-06-19
---

# System Overview

A recreation center management system.
`)

	c, err := ParseConcept(data, "clients/mottpark.md")
	if err != nil {
		t.Fatal(err)
	}

	if c.Meta.Type != "Client Profile" {
		t.Errorf("type = %q, want %q", c.Meta.Type, "Client Profile")
	}
	if c.Meta.Title != "Mott Park Recreation" {
		t.Errorf("title = %q, want %q", c.Meta.Title, "Mott Park Recreation")
	}
	if c.Meta.Resource != "https://mottpark.civic-os.org" {
		t.Errorf("resource = %q", c.Meta.Resource)
	}
	if len(c.Meta.Tags) != 3 {
		t.Errorf("tags = %v, want 3 tags", c.Meta.Tags)
	}
	if c.Meta.Timestamp != "2026-06-19" {
		t.Errorf("timestamp = %q", c.Meta.Timestamp)
	}
	if c.Path != "clients/mottpark.md" {
		t.Errorf("path = %q", c.Path)
	}
	if !strings.Contains(c.Body, "# System Overview") {
		t.Errorf("body missing expected content: %q", c.Body)
	}
}

func TestParseConceptMissingType(t *testing.T) {
	data := []byte(`---
title: No Type
---

Body content.
`)
	_, err := ParseConcept(data, "test.md")
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	if !strings.Contains(err.Error(), "missing required field 'type'") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseConceptMinimal(t *testing.T) {
	data := []byte(`---
type: Note
---
`)
	c, err := ParseConcept(data, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if c.Meta.Type != "Note" {
		t.Errorf("type = %q", c.Meta.Type)
	}
	if c.Meta.Title != "" {
		t.Errorf("title should be empty, got %q", c.Meta.Title)
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	original := &Concept{
		Meta: ConceptMeta{
			Type:        "Decision Record",
			Title:       "Use Go for MCP Server",
			Description: "Go provides small binaries and built-in concurrency.",
			Tags:        []string{"architecture", "infrastructure"},
			Timestamp:   "2026-06-19",
		},
		Body: "# Context\n\nWe needed to choose an implementation language.",
		Path: "decisions/use-go.md",
	}

	data, err := SerializeConcept(original)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseConcept(data, original.Path)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Meta.Type != original.Meta.Type {
		t.Errorf("type = %q, want %q", parsed.Meta.Type, original.Meta.Type)
	}
	if parsed.Meta.Title != original.Meta.Title {
		t.Errorf("title = %q, want %q", parsed.Meta.Title, original.Meta.Title)
	}
	if parsed.Meta.Description != original.Meta.Description {
		t.Errorf("description mismatch")
	}
	if len(parsed.Meta.Tags) != len(original.Meta.Tags) {
		t.Errorf("tags = %v, want %v", parsed.Meta.Tags, original.Meta.Tags)
	}
	if !strings.Contains(parsed.Body, "# Context") {
		t.Errorf("body missing content: %q", parsed.Body)
	}
}

func TestSerializeWithResource(t *testing.T) {
	c := &Concept{
		Meta: ConceptMeta{
			Type:     "Client Profile",
			Title:    "Test Client",
			Resource: "https://test.civic-os.org",
		},
		Body: "Body text.",
		Path: "clients/test.md",
	}

	data, err := SerializeConcept(c)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), "resource: https://test.civic-os.org") {
		t.Errorf("serialized data missing resource: %s", data)
	}
}

func TestParseTemplates(t *testing.T) {
	// Parse all template files as fixtures
	templatesDir := filepath.Join("..", "..", "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		t.Skipf("templates directory not found: %v", err)
	}

	parsed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(templatesDir, e.Name()))
		if err != nil {
			t.Errorf("read %s: %v", e.Name(), err)
			continue
		}

		c, err := ParseConcept(data, e.Name())
		if err != nil {
			// Some templates (like index.md) may not have frontmatter
			t.Logf("skip %s: %v", e.Name(), err)
			continue
		}

		if c.Meta.Type == "" {
			t.Errorf("%s: type is empty", e.Name())
		}
		parsed++
	}

	if parsed == 0 {
		t.Error("no templates were successfully parsed")
	}
	t.Logf("parsed %d template files", parsed)
}
