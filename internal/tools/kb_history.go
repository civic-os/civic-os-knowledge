package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type HistoryInput struct {
	Path string `json:"path" jsonschema:"Relative file path of the concept"`
}

func HistoryHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *HistoryInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *HistoryInput) (*mcp.CallToolResult, any, error) {
		versions, err := deps.Bundle.History(input.Path)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("history failed: %w", err))
			return result, nil, nil
		}

		if len(versions) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("No version history for %s.", input.Path)},
				},
			}, nil, nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Version history for %s (%d snapshot(s)):\n\n", input.Path, len(versions)))
		for _, v := range versions {
			sb.WriteString(fmt.Sprintf("- version %d\n", v))
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	}
}

func HistoryTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_history",
		Description: "List version history for a concept. Each version number represents a snapshot taken before an update. Use a version number with kb_diff to see what changed.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
