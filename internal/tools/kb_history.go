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
		path := cleanPath(input.Path)
		current, viaAlias := deps.Bundle.Resolve(path)

		versions, err := deps.Bundle.History(current)
		if err != nil {
			return errorResult("history failed: %w", err), nil, nil
		}

		var sb strings.Builder
		if viaAlias {
			sb.WriteString(movedHeader(path, current))
		}
		if len(versions) == 0 {
			sb.WriteString(fmt.Sprintf("No version history for %s.", current))
		} else {
			sb.WriteString(fmt.Sprintf("Version history for %s (%d snapshot(s)):\n\n", current, len(versions)))
			for _, v := range versions {
				sb.WriteString(fmt.Sprintf("- version %d\n", v))
			}
		}
		if c, err := deps.Bundle.Read(current); err == nil && len(c.Meta.Aliases) > 0 {
			sb.WriteString("\nPreviously at: " + strings.Join(c.Meta.Aliases, ", ") + "\n")
		}

		return textResult(sb.String()), nil, nil
	}
}

func HistoryTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_history",
		Description: "List version history for a concept. Each version number represents a snapshot taken before an update. Use a version number with kb_diff to see what changed. History follows a concept when it moves; former paths are listed as \"Previously at\".",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
