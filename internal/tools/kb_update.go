package tools

import (
	"context"
	"fmt"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type UpdateInput struct {
	Path        string   `json:"path" jsonschema:"Relative file path of the concept to update"`
	Type        string   `json:"type,omitempty" jsonschema:"Updated concept type"`
	Title       string   `json:"title,omitempty" jsonschema:"Updated title"`
	Description string   `json:"description,omitempty" jsonschema:"Updated description"`
	Resource    string   `json:"resource,omitempty" jsonschema:"Updated resource URL"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Updated tags"`
	Body        string   `json:"body,omitempty" jsonschema:"Updated markdown body content"`
}

func UpdateHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *UpdateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *UpdateInput) (*mcp.CallToolResult, any, error) {
		// Read existing to merge fields
		existing, err := deps.Bundle.Read(input.Path)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("concept not found: %w", err))
			return result, nil, nil
		}

		// Merge: only update fields that are provided
		if input.Type != "" {
			existing.Meta.Type = input.Type
		}
		if input.Title != "" {
			existing.Meta.Title = input.Title
		}
		if input.Description != "" {
			existing.Meta.Description = input.Description
		}
		if input.Resource != "" {
			existing.Meta.Resource = input.Resource
		}
		if input.Tags != nil {
			existing.Meta.Tags = input.Tags
		}
		if input.Body != "" {
			existing.Body = input.Body
		}
		existing.Meta.Timestamp = bundle.NowTimestamp()

		if err := deps.Bundle.Update(existing); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("update failed: %w", err))
			return result, nil, nil
		}

		deps.Index.Add(existing)
		deps.onWrite(input.Path)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Updated concept: %s", input.Path)},
			},
		}, nil, nil
	}
}

func UpdateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_update",
		Description: "Update an existing knowledge concept. Only specified fields are changed; others are preserved. A version snapshot is created before updating.",
	}
}
