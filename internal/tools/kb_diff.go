package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DiffInput struct {
	Path      string `json:"path" jsonschema:"Relative file path of the concept"`
	Timestamp string `json:"timestamp" jsonschema:"Version timestamp to diff against (from kb_history)"`
}

func DiffHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *DiffInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *DiffInput) (*mcp.CallToolResult, any, error) {
		diff, err := deps.Bundle.Diff(input.Path, input.Timestamp)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("diff failed: %w", err))
			return result, nil, nil
		}

		if diff == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No differences found."},
				},
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Diff of %s (version %s vs current):\n\n%s", input.Path, input.Timestamp, diff)},
			},
		}, nil, nil
	}
}

func DiffTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_diff",
		Description: "Show differences between a version snapshot and the current content of a concept.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
