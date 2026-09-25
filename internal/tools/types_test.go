package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// seed writes a raw concept file, bypassing the tools, as older concepts were.
func seed(t *testing.T, deps *Deps, path, conceptType string) {
	t.Helper()
	full := filepath.Join(deps.Bundle.RootDir(), path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\ntype: " + conceptType + "\ntitle: Seeded\n---\n\nBody.\n"
	if err := os.WriteFile(full, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFolderDecidesTypeOnCreate(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	create := CreateHandler(deps)

	result, _, _ := create(ctx, req, &CreateInput{Path: "marketing/voice.md", Title: "Voice"})
	if text := contentText(result); !strings.Contains(text, "(Marketing Document, version: 1)") {
		t.Errorf("type should be inferred from the folder: %s", text)
	}

	result, _, _ = create(ctx, req, &CreateInput{Path: "strategy/voice.md", Type: "Marketing Document", Title: "Voice"})
	if result.GetError() == nil || !strings.Contains(result.GetError().Error(), "Marketing Document concepts live in marketing/") {
		t.Errorf("mismatched type should be rejected with the right folder: %v", result.GetError())
	}

	result, _, _ = create(ctx, req, &CreateInput{Path: "notes/voice.md", Title: "Voice"})
	if result.GetError() == nil || !strings.Contains(result.GetError().Error(), "not in a registered folder") {
		t.Errorf("unregistered folder should be rejected: %v", result.GetError())
	}
}

func TestFolderDecidesTypeOnUpdate(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	update := UpdateHandler(deps)
	CreateHandler(deps)(ctx, req, &CreateInput{Path: "strategy/voice.md", Title: "Voice"})

	result, _, _ := update(ctx, req, &UpdateInput{Path: "strategy/voice.md", Type: "Marketing Document"})
	if result.GetError() == nil || !strings.Contains(result.GetError().Error(), "move it into that type's folder with kb_move") {
		t.Errorf("recategorizing via kb_update should point to kb_move: %v", result.GetError())
	}

	// Older concepts with a stray type can be fixed in place, and warn until they are.
	seed(t, deps, "infrastructure/og.md", "Infrastructure")
	result, _, _ = update(ctx, req, &UpdateInput{Path: "infrastructure/og.md", Title: "OG"})
	if text := contentText(result); !strings.Contains(text, `Type warning: infrastructure/og.md has type "Infrastructure"`) {
		t.Errorf("expected type warning: %s", text)
	}
	result, _, _ = update(ctx, req, &UpdateInput{Path: "infrastructure/og.md", Type: "infrastructure component"})
	if text := contentText(result); result.GetError() != nil || strings.Contains(text, "Type warning") {
		t.Errorf("fixing the type in place should succeed cleanly: %v %s", result.GetError(), text)
	}
	read, _, _ := ReadHandler(deps)(ctx, req, &ReadInput{Path: "infrastructure/og.md"})
	if !strings.Contains(contentText(read), "type: Infrastructure Component") {
		t.Errorf("type not canonicalized: %s", contentText(read))
	}
}

func TestMoveSetsTypeAndAuditReportsMismatches(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	CreateHandler(deps)(ctx, req, &CreateInput{Path: "strategy/voice.md", Title: "Voice"})
	seed(t, deps, "competitors/vendor.md", "Competitive Analysis")
	seed(t, deps, "strategy/pricing.md", "Pricing Model")

	result, _, _ := LinksHandler(deps)(ctx, req, &LinksInput{})
	text := contentText(result)
	for _, want := range []string{
		"Folder/type mismatches (2):",
		"competitors/vendor.md is outside the registered folders; move it to competitive-analysis/vendor.md",
		`strategy/pricing.md has type "Pricing Model" but strategy/ holds Strategy Document concepts`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("audit missing %q:\n%s", want, text)
		}
	}

	result, _, _ = MoveHandler(deps)(ctx, req, &MoveInput{Moves: []MoveItemInput{{Path: "strategy/voice.md", NewPath: "branding/voice.md"}}})
	if result.GetError() == nil || !strings.Contains(result.GetError().Error(), "not in a registered folder") {
		t.Errorf("move into an unregistered folder should fail: %v", result.GetError())
	}

	result, _, _ = MoveHandler(deps)(ctx, req, &MoveInput{Moves: []MoveItemInput{
		{Path: "strategy/voice.md", NewPath: "marketing/voice.md"},
		{Path: "competitors/vendor.md", NewPath: "competitive-analysis/vendor.md"},
	}})
	text = contentText(result)
	if !strings.Contains(text, "strategy/voice.md → marketing/voice.md (version 1 → 2, type Strategy Document → Marketing Document") ||
		!strings.Contains(text, "competitors/vendor.md → competitive-analysis/vendor.md (version 1 → 2, 0 snapshot(s)") {
		t.Errorf("move result:\n%s", text)
	}
}
