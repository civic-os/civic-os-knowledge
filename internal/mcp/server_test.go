package mcpserver

import (
	"context"
	"testing"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/civic-os/civic-os-knowledge/internal/search"
	"github.com/civic-os/civic-os-knowledge/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	dir := t.TempDir()
	b, err := bundle.NewBundle(dir + "/bundle")
	if err != nil {
		t.Fatal(err)
	}

	idx := search.NewIndex()
	deps := &tools.Deps{Bundle: b, Index: idx}
	server := NewMCPServer(deps)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()
	go server.Connect(ctx, serverTransport, nil)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.1.0",
	}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}

	return cs
}

func TestListTools(t *testing.T) {
	cs := testSession(t)
	ctx := context.Background()

	resp, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Tools) != 7 {
		t.Errorf("got %d tools, want 7", len(resp.Tools))
		for _, tool := range resp.Tools {
			t.Logf("  tool: %s", tool.Name)
		}
	}

	expected := map[string]bool{
		"kb_read":    false,
		"kb_search":  false,
		"kb_list":    false,
		"kb_create":  false,
		"kb_update":  false,
		"kb_history": false,
		"kb_diff":    false,
	}

	for _, tool := range resp.Tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestCallCreateAndRead(t *testing.T) {
	cs := testSession(t)
	ctx := context.Background()

	// Create
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "kb_create",
		Arguments: map[string]any{
			"path":  "clients/test.md",
			"type":  "Client Profile",
			"title": "Test Client",
			"body":  "Test body content.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetError() != nil {
		t.Fatalf("create error: %v", result.GetError())
	}

	// Read
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_read",
		Arguments: map[string]any{"path": "clients/test.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetError() != nil {
		t.Fatalf("read error: %v", result.GetError())
	}
	text := extractText(result)
	if text == "" {
		t.Fatal("empty read result")
	}
}

func TestCallSearch(t *testing.T) {
	cs := testSession(t)
	ctx := context.Background()

	cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "kb_create",
		Arguments: map[string]any{
			"path":  "clients/alpha.md",
			"type":  "Client Profile",
			"title": "Alpha Corporation",
			"body":  "Manages community centers.",
		},
	})

	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_search",
		Arguments: map[string]any{"query": "alpha"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := extractText(result)
	if text == "" {
		t.Fatal("empty search result")
	}
}

func TestCallList(t *testing.T) {
	cs := testSession(t)
	ctx := context.Background()

	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := extractText(result)
	if text == "" {
		t.Fatal("empty list result")
	}
}

func extractText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}
