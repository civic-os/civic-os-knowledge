package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// recordHooks makes deps log every hook call as "kind:path", in order.
func recordHooks(deps *Deps) *[]string {
	var calls []string
	deps.OnWrite = func(p string) { calls = append(calls, "write:"+p) }
	deps.OnSnapshot = func(p string) { calls = append(calls, "snapshot:"+p) }
	deps.OnDelete = func(p string) { calls = append(calls, "delete:"+p) }
	deps.OnSnapshotDelete = func(p string) { calls = append(calls, "snapshot-delete:"+p) }
	return &calls
}

func TestMoveToolRoundTrip(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	create := CreateHandler(deps)
	update := UpdateHandler(deps)
	create(ctx, req, &CreateInput{Path: "strategy/brand-voice.md", Type: "Strategy Document", Title: "Brand Voice", Body: "Voice."})
	update(ctx, req, &UpdateInput{Path: "strategy/brand-voice.md", Body: "Voice v2."})
	create(ctx, req, &CreateInput{Path: "runbooks/website-guide.md", Type: "Runbook", Title: "Website Guide",
		Body: "Follow [Brand Voice](/strategy/brand-voice.md). Also `strategy/brand-voice.md`."})

	calls := recordHooks(deps)
	result, _, _ := MoveHandler(deps)(ctx, req, &MoveInput{Moves: []MoveItemInput{
		{Path: "/strategy/brand-voice.md", NewPath: "marketing/brand-voice.md", Version: 2},
	}})
	if result.GetError() != nil {
		t.Fatalf("move failed: %v", result.GetError())
	}
	text := contentText(result)
	for _, want := range []string{
		"strategy/brand-voice.md → marketing/brand-voice.md (version 2 → 3, 1 snapshot(s) carried)",
		"runbooks/website-guide.md (version 2)",
		"- strategy/brand-voice.md: runbooks/website-guide.md",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("result missing %q:\n%s", want, text)
		}
	}

	// Writes and snapshot writes reach the hooks before any deletes.
	lastWrite, firstDelete := -1, len(*calls)
	for i, c := range *calls {
		if strings.HasPrefix(c, "write:") || strings.HasPrefix(c, "snapshot:") {
			lastWrite = i
		} else if i < firstDelete {
			firstDelete = i
		}
	}
	if lastWrite > firstDelete {
		t.Errorf("a delete hook fired before a write hook: %v", *calls)
	}
	for _, want := range []string{"write:marketing/brand-voice.md", "write:runbooks/website-guide.md",
		"delete:strategy/brand-voice.md", "snapshot-delete:strategy/brand-voice/brand-voice.1.md"} {
		if !strings.Contains(strings.Join(*calls, " "), want) {
			t.Errorf("hooks missing %s: %v", want, *calls)
		}
	}

	// The search index only knows the new path.
	search, _, _ := SearchHandler(deps)(ctx, req, &SearchInput{Query: "voice"})
	if text := contentText(search); !strings.Contains(text, "marketing/brand-voice.md") || strings.Contains(text, "`strategy/brand-voice.md`") {
		t.Errorf("search after move:\n%s", text)
	}
	list, _, _ := ListHandler(deps)(ctx, req, &ListInput{})
	if text := contentText(list); strings.Contains(text, "`strategy/") || !strings.Contains(text, "`marketing/brand-voice.md` — Brand Voice (Strategy Document) [v3]") {
		t.Errorf("list after move:\n%s", text)
	}

	// Reads of the old path resolve with a header; writes are refused.
	read, _, _ := ReadHandler(deps)(ctx, req, &ReadInput{Path: "strategy/brand-voice.md"})
	if text := contentText(read); !strings.Contains(text, "[moved: strategy/brand-voice.md → marketing/brand-voice.md") ||
		!strings.Contains(text, "aliases: [strategy/brand-voice.md]") {
		t.Errorf("read via alias:\n%s", text)
	}
	upd, _, _ := update(ctx, req, &UpdateInput{Path: "strategy/brand-voice.md", Title: "X"})
	if upd.GetError() == nil || !strings.Contains(upd.GetError().Error(), "moved to marketing/brand-voice.md") {
		t.Errorf("update via alias should be refused: %v", upd.GetError())
	}

	hist, _, _ := HistoryHandler(deps)(ctx, req, &HistoryInput{Path: "marketing/brand-voice.md"})
	if text := contentText(hist); !strings.Contains(text, "2 snapshot(s)") || !strings.Contains(text, "Previously at: strategy/brand-voice.md") {
		t.Errorf("history:\n%s", text)
	}
	diff, _, _ := DiffHandler(deps)(ctx, req, &DiffInput{Path: "strategy/brand-voice.md", Version: 2})
	if text := contentText(diff); !strings.Contains(text, "+aliases: [strategy/brand-voice.md]") || !strings.Contains(text, "[moved:") {
		t.Errorf("diff via alias:\n%s", text)
	}
}

func TestMoveToolRejectsWholeBatch(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	create := CreateHandler(deps)
	create(ctx, req, &CreateInput{Path: "a.md", Type: "Note", Title: "A"})
	create(ctx, req, &CreateInput{Path: "b.md", Type: "Note", Title: "B"})

	result, _, _ := MoveHandler(deps)(ctx, req, &MoveInput{Moves: []MoveItemInput{
		{Path: "a.md", NewPath: "x/a.md"},
		{Path: "b.md", NewPath: "x/b.md", Version: 7},
	}})
	if result.GetError() == nil || !strings.Contains(result.GetError().Error(), "Conflict") {
		t.Fatalf("expected conflict: %v", result.GetError())
	}
	if read, _, _ := ReadHandler(deps)(ctx, req, &ReadInput{Path: "a.md"}); read.GetError() != nil {
		t.Error("a.md must not move when the batch is rejected")
	}
}

func TestCreateLinkChecksTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	create := CreateHandler(deps)
	create(ctx, req, &CreateInput{Path: "prospects/globex-jane-doe.md", Type: "Prospect", Title: "Globex"})

	result, _, _ := create(ctx, req, &CreateInput{Path: "company/team.md", Type: "Company Reference", Title: "Team",
		Body: "Advisor at [Globex](/partners/globex.md)."})
	if result.GetError() == nil || !strings.Contains(result.GetError().Error(), "did you mean prospects/globex-jane-doe.md") {
		t.Fatalf("expected rejection with suggestion: %v", result.GetError())
	}

	result, _, _ = create(ctx, req, &CreateInput{Path: "company/team.md", Type: "Company Reference", Title: "Team",
		Body: "Advisor at [Globex](../prospects/globex-jane-doe.md), see [site](chrome://settings)."})
	if result.GetError() != nil {
		t.Fatalf("create failed: %v", result.GetError())
	}
	if text := contentText(result); !strings.Contains(text, "../prospects/globex-jane-doe.md → /prospects/globex-jane-doe.md") {
		t.Errorf("expected normalization note:\n%s", text)
	}

	result, _, _ = create(ctx, req, &CreateInput{Path: "../escape.md", Type: "Note", Title: "X"})
	if result.GetError() == nil {
		t.Error("create outside the bundle should fail")
	}
}

func TestLinksTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	create := CreateHandler(deps)
	create(ctx, req, &CreateInput{Path: "strategy/foo.md", Type: "Note", Title: "Foo"})
	create(ctx, req, &CreateInput{Path: "clients/a.md", Type: "Note", Title: "A", Body: "[Foo](/strategy/foo.md) ×2 [Foo](/strategy/foo.md)"})

	result, _, _ := LinksHandler(deps)(ctx, req, &LinksInput{Path: "strategy/foo.md"})
	if text := contentText(result); !strings.Contains(text, "Inbound links (1):\n- clients/a.md ×2") {
		t.Errorf("links:\n%s", text)
	}

	result, _, _ = LinksHandler(deps)(ctx, req, &LinksInput{})
	if text := contentText(result); !strings.Contains(text, "Link audit: 2 concepts, 2 concept links") || !strings.Contains(text, "Broken links (0)") {
		t.Errorf("audit:\n%s", text)
	}

	create(ctx, req, &CreateInput{Path: "strategy/brand-voice.md", Type: "Note", Title: "Voice"})
	result, _, _ = LinksHandler(deps)(ctx, req, &LinksInput{Path: "strategy/brand.md"})
	if result.GetError() == nil || !strings.Contains(result.GetError().Error(), "did you mean strategy/brand-voice.md") {
		t.Errorf("missing concept should suggest: %v", result.GetError())
	}
}
