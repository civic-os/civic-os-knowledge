package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchInput struct {
	Query string   `json:"query" jsonschema:"Search query text (case-insensitive substring match)"`
	Type  string   `json:"type,omitempty" jsonschema:"Filter by concept type (e.g. Client Profile)"`
	Tags  []string `json:"tags,omitempty" jsonschema:"Filter by tags (returns concepts matching any tag)"`
}

func SearchHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *SearchInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *SearchInput) (*mcp.CallToolResult, any, error) {
		results := deps.Index.Search(input.Query, input.Type, input.Tags)

		if len(results) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No concepts found matching the search criteria."},
				},
			}, nil, nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d concept(s):\n\n", len(results)))
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`)\n  Type: %s", r.Meta.Title, r.Path, r.Meta.Type))
			if r.Meta.Description != "" {
				sb.WriteString(fmt.Sprintf("\n  %s", r.Meta.Description))
			}
			sb.WriteString("\n\n")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: sb.String()},
			},
		}, nil, nil
	}
}

func SearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_search",
		Description: `Search knowledge concepts by query text, type, and/or tags. Returns matching concepts ranked by relevance.

Search the knowledgebase before answering questions about Civic OS clients, instances, deployments, infrastructure, or business operations. Don't guess at facts the KB already captures.`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
