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
		path := cleanPath(input.Path)
		c, err := deps.Bundle.Read(path)
		if err != nil {
			return notFoundResult(deps, "read", path, err), nil, nil
		}

		data, err := bundle.SerializeConcept(c)
		if err != nil {
			return errorResult("serialize failed: %w", err), nil, nil
		}

		text := fmt.Sprintf("[version: %d]\n", c.Version)
		if c.ResolvedFrom != "" {
			text += movedHeader(c.ResolvedFrom, c.Path)
		}
		return textResult(text + string(data)), nil, nil
	}
}

func ReadTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_read",
		Description: "Read a knowledge concept by its file path. Returns the full markdown content including YAML frontmatter, prefixed with [version: N]. Pass the version number to kb_update to enable optimistic concurrency control. Reading a path a concept has moved away from returns the concept at its current path, with a [moved: old → new] line; its former paths are listed under `aliases` in the frontmatter.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
