package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/civic-os/civic-os-knowledge/internal/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testDeps(t *testing.T) *Deps {
	t.Helper()
	dir := t.TempDir()
	b, err := bundle.NewBundle(dir + "/bundle")
	if err != nil {
		t.Fatal(err)
	}

	idx := search.NewIndex()

	return &Deps{
		Bundle: b,
		Index:  idx,
		OnWrite: func(path string) {
		},
		OnSnapshot: func(snapshotRelPath string) {
		},
	}
}

func TestCreateAndReadTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	// Create
	createFn := CreateHandler(deps)
	result, _, err := createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{
		Path:        "clients/test.md",
		Type:        "Client Profile",
		Title:       "Test Client",
		Description: "A test client for testing.",
		Tags:        []string{"test"},
		Body:        "# Overview\n\nTest body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := contentText(result)
	if !strings.Contains(text, "Created") {
		t.Errorf("unexpected result: %s", text)
	}
	if !strings.Contains(text, "version: 1") {
		t.Errorf("expected version 1 in create result: %s", text)
	}

	// Read
	readFn := ReadHandler(deps)
	result, _, err = readFn(ctx, &mcp.CallToolRequest{}, &ReadInput{Path: "clients/test.md"})
	if err != nil {
		t.Fatal(err)
	}
	text = contentText(result)
	if !strings.Contains(text, "Test Client") {
		t.Errorf("read result missing title: %s", text)
	}
	if !strings.Contains(text, "Test body.") {
		t.Errorf("read result missing body: %s", text)
	}
	if !strings.Contains(text, "[version: 1]") {
		t.Errorf("read result missing version prefix: %s", text)
	}
}

func TestReadNotFound(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	readFn := ReadHandler(deps)
	result, _, err := readFn(ctx, &mcp.CallToolRequest{}, &ReadInput{Path: "missing.md"})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetError() == nil {
		t.Error("expected tool error for missing file")
	}
}

func TestCreateDuplicateTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	createFn := CreateHandler(deps)
	input := &CreateInput{Path: "a.md", Type: "Note", Title: "A"}
	createFn(ctx, &mcp.CallToolRequest{}, input)

	result, _, _ := createFn(ctx, &mcp.CallToolRequest{}, input)
	if result.GetError() == nil {
		t.Error("expected error for duplicate create")
	}
}

func TestSearchTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	createFn := CreateHandler(deps)
	createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{
		Path:  "clients/a.md",
		Type:  "Client Profile",
		Title: "Alpha Client",
		Body:  "Manages alpha systems.",
	})
	createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{
		Path:  "runbooks/deploy.md",
		Type:  "Runbook",
		Title: "Deploy Procedure",
		Body:  "Step 1: deploy alpha.",
	})

	searchFn := SearchHandler(deps)
	result, _, err := searchFn(ctx, &mcp.CallToolRequest{}, &SearchInput{Query: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	text := contentText(result)
	if !strings.Contains(text, "Alpha Client") {
		t.Errorf("search missing Alpha Client: %s", text)
	}
	if !strings.Contains(text, "[v1]") {
		t.Errorf("search result missing version: %s", text)
	}
}

func TestSearchNoResults(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	searchFn := SearchHandler(deps)
	result, _, _ := searchFn(ctx, &mcp.CallToolRequest{}, &SearchInput{Query: "nonexistent"})
	text := contentText(result)
	if !strings.Contains(text, "No concepts found") {
		t.Errorf("expected no results message: %s", text)
	}
}

func TestListTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	createFn := CreateHandler(deps)
	createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{Path: "a.md", Type: "Note", Title: "A"})
	createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{Path: "b.md", Type: "Runbook", Title: "B"})

	listFn := ListHandler(deps)

	// List all
	result, _, _ := listFn(ctx, &mcp.CallToolRequest{}, &ListInput{})
	text := contentText(result)
	if !strings.Contains(text, "2 concept(s)") {
		t.Errorf("expected 2 concepts: %s", text)
	}
	if !strings.Contains(text, "[v1]") {
		t.Errorf("list result missing version: %s", text)
	}

	// List by type
	result, _, _ = listFn(ctx, &mcp.CallToolRequest{}, &ListInput{Type: "Note"})
	text = contentText(result)
	if !strings.Contains(text, "1 concept(s)") {
		t.Errorf("expected 1 concept of type Note: %s", text)
	}
}

func TestListEmpty(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	listFn := ListHandler(deps)
	result, _, _ := listFn(ctx, &mcp.CallToolRequest{}, &ListInput{})
	text := contentText(result)
	if !strings.Contains(text, "No concepts") {
		t.Errorf("expected empty message: %s", text)
	}
}

func TestUpdateTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	createFn := CreateHandler(deps)
	createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{
		Path:  "a.md",
		Type:  "Note",
		Title: "Original",
		Body:  "Original body.",
	})

	updateFn := UpdateHandler(deps)
	result, _, err := updateFn(ctx, &mcp.CallToolRequest{}, &UpdateInput{
		Path:  "a.md",
		Title: "Updated",
		Body:  "Updated body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := contentText(result)
	if !strings.Contains(text, "Updated concept") {
		t.Errorf("unexpected result: %s", text)
	}
	if !strings.Contains(text, "version: 2") {
		t.Errorf("expected version 2 in update result: %s", text)
	}

	// Verify update
	readFn := ReadHandler(deps)
	result, _, _ = readFn(ctx, &mcp.CallToolRequest{}, &ReadInput{Path: "a.md"})
	text = contentText(result)
	if !strings.Contains(text, "title: Updated") {
		t.Errorf("title not updated: %s", text)
	}
	// Type should be preserved
	if !strings.Contains(text, "type: Note") {
		t.Errorf("type not preserved: %s", text)
	}
	if !strings.Contains(text, "[version: 2]") {
		t.Errorf("read should show version 2: %s", text)
	}
}

func TestUpdateNotFound(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	updateFn := UpdateHandler(deps)
	result, _, _ := updateFn(ctx, &mcp.CallToolRequest{}, &UpdateInput{Path: "missing.md", Title: "X"})
	if result.GetError() == nil {
		t.Error("expected error for missing file")
	}
}

func TestUpdateConflictTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	createFn := CreateHandler(deps)
	createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{
		Path:  "a.md",
		Type:  "Note",
		Title: "V1",
		Body:  "Body v1.",
	})

	updateFn := UpdateHandler(deps)

	// Update with version 1 succeeds
	result, _, _ := updateFn(ctx, &mcp.CallToolRequest{}, &UpdateInput{
		Path:    "a.md",
		Title:   "V2",
		Body:    "Body v2.",
		Version: 1,
	})
	if result.GetError() != nil {
		t.Fatalf("first update should succeed: %v", result.GetError())
	}

	// Update with stale version 1 fails with conflict
	result, _, _ = updateFn(ctx, &mcp.CallToolRequest{}, &UpdateInput{
		Path:    "a.md",
		Title:   "V2-stale",
		Body:    "Body v2 stale.",
		Version: 1,
	})
	if result.GetError() == nil {
		t.Fatal("expected conflict error for stale version")
	}
	errText := result.GetError().Error()
	if !strings.Contains(errText, "Conflict") {
		t.Errorf("expected Conflict in error: %s", errText)
	}

	// Update without version (0) always succeeds
	result, _, _ = updateFn(ctx, &mcp.CallToolRequest{}, &UpdateInput{
		Path:  "a.md",
		Title: "V3",
		Body:  "Body v3.",
	})
	if result.GetError() != nil {
		t.Fatalf("update without version should succeed: %v", result.GetError())
	}
	text := contentText(result)
	if !strings.Contains(text, "version: 3") {
		t.Errorf("expected version 3: %s", text)
	}
}

func TestHistoryTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	createFn := CreateHandler(deps)
	createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{Path: "a.md", Type: "Note", Title: "V1"})

	updateFn := UpdateHandler(deps)
	updateFn(ctx, &mcp.CallToolRequest{}, &UpdateInput{Path: "a.md", Title: "V2"})

	histFn := HistoryHandler(deps)
	result, _, err := histFn(ctx, &mcp.CallToolRequest{}, &HistoryInput{Path: "a.md"})
	if err != nil {
		t.Fatal(err)
	}
	text := contentText(result)
	if !strings.Contains(text, "1 snapshot(s)") {
		t.Errorf("expected 1 snapshot: %s", text)
	}
	if !strings.Contains(text, "version 1") {
		t.Errorf("expected version 1 in history: %s", text)
	}
}

func TestHistoryEmpty(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	histFn := HistoryHandler(deps)
	result, _, _ := histFn(ctx, &mcp.CallToolRequest{}, &HistoryInput{Path: "a.md"})
	text := contentText(result)
	if !strings.Contains(text, "No version history") {
		t.Errorf("expected no history: %s", text)
	}
}

func TestDiffTool(t *testing.T) {
	deps := testDeps(t)
	ctx := context.Background()

	createFn := CreateHandler(deps)
	createFn(ctx, &mcp.CallToolRequest{}, &CreateInput{Path: "a.md", Type: "Note", Title: "V1", Body: "Body v1."})

	updateFn := UpdateHandler(deps)
	updateFn(ctx, &mcp.CallToolRequest{}, &UpdateInput{Path: "a.md", Title: "V2", Body: "Body v2."})

	// Diff against version 1
	diffFn := DiffHandler(deps)
	result, _, err := diffFn(ctx, &mcp.CallToolRequest{}, &DiffInput{Path: "a.md", Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	text := contentText(result)
	if !strings.Contains(text, "Diff of") {
		t.Errorf("expected diff output: %s", text)
	}
	if !strings.Contains(text, "version 1") {
		t.Errorf("expected version 1 in diff header: %s", text)
	}
}

func contentText(r *mcp.CallToolResult) string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}
