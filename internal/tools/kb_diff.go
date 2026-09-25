package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DiffInput struct {
	Path    string `json:"path" jsonschema:"Relative file path of the concept"`
	Version int    `json:"version" jsonschema:"Version number to diff against (from kb_history)"`
}

func DiffHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *DiffInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *DiffInput) (*mcp.CallToolResult, any, error) {
		path := cleanPath(input.Path)
		current, viaAlias := deps.Bundle.Resolve(path)

		diff, err := deps.Bundle.Diff(current, input.Version)
		if err != nil {
			return errorResult("diff failed: %w", err), nil, nil
		}

		header := ""
		if viaAlias {
			header = movedHeader(path, current)
		}
		if diff == "" {
			return textResult(header + "No differences found."), nil, nil
		}
		return textResult(header + fmt.Sprintf("Diff of %s (version %d vs current):\n\n%s", current, input.Version, diff)), nil, nil
	}
}

func DiffTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_diff",
		Description: "Show a unified diff between a version snapshot and the current content of a concept.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
