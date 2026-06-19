package tools

import (
	"context"
	"fmt"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ReadInput struct {
	Path string `json:"path" jsonschema:"Relative path to the concept file (e.g. clients/mottpark.md)"`
}

func ReadHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *ReadInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *ReadInput) (*mcp.CallToolResult, any, error) {
		c, err := deps.Bundle.Read(input.Path)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("read failed: %w", err))
			return result, nil, nil
		}

		data, err := bundle.SerializeConcept(c)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("serialize failed: %w", err))
			return result, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(data)},
			},
		}, nil, nil
	}
}

func ReadTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_read",
		Description: "Read a knowledge concept by its file path. Returns the full markdown content including YAML frontmatter.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
