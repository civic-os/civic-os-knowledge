package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListInput struct {
	Type string `json:"type,omitempty" jsonschema:"Filter by concept type (e.g. Runbook, Client Profile)"`
}

func ListHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *ListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *ListInput) (*mcp.CallToolResult, any, error) {
		results := deps.Index.List(input.Type)

		if len(results) == 0 {
			msg := "No concepts in the knowledgebase."
			if input.Type != "" {
				msg = fmt.Sprintf("No concepts of type %q found.", input.Type)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: msg},
				},
			}, nil, nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d concept(s)", len(results)))
		if input.Type != "" {
			sb.WriteString(fmt.Sprintf(" of type %q", input.Type))
		}
		sb.WriteString(":\n\n")

		for _, r := range results {
			sb.WriteString(fmt.Sprintf("- `%s` — %s (%s)\n", r.Path, r.Meta.Title, r.Meta.Type))
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	}
}

func ListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_list",
		Description: "List all knowledge concepts, optionally filtered by type.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
