package tools

import (
	"context"
	"fmt"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateInput struct {
	Path        string   `json:"path" jsonschema:"Relative file path for the new concept (e.g. clients/newclient.md)"`
	Type        string   `json:"type" jsonschema:"Concept type: Client Profile, Instance Deployment, Project Specification, Decision Record, Runbook, Strategy Document, Research Analysis, Infrastructure Component, Proposal, or Meeting Note"`
	Title       string   `json:"title" jsonschema:"Human-readable title"`
	Description string   `json:"description,omitempty" jsonschema:"One-sentence description"`
	Resource    string   `json:"resource,omitempty" jsonschema:"External resource URL"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Tags for categorization"`
	Body        string   `json:"body,omitempty" jsonschema:"Markdown body content"`
}

func CreateHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *CreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *CreateInput) (*mcp.CallToolResult, any, error) {
		c := &bundle.Concept{
			Meta: bundle.ConceptMeta{
				Type:        input.Type,
				Title:       input.Title,
				Description: input.Description,
				Resource:    input.Resource,
				Tags:        input.Tags,
				Timestamp:   bundle.NowTimestamp(),
			},
			Body: input.Body,
			Path: input.Path,
		}

		if err := deps.Bundle.Create(c); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("create failed: %w", err))
			return result, nil, nil
		}

		deps.Index.Add(c)
		deps.onWrite(input.Path)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Created concept: %s (%s)", input.Path, input.Type)},
			},
		}, nil, nil
	}
}

func CreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_create",
		Description: `Create a new knowledge concept. Each concept should capture one idea, decision, or artifact — prefer creating a new linked concept over expanding an existing one. Cross-link related concepts with markdown paths (e.g. [Client Profile](/clients/neh.md)).

Concept types: Client Profile, Instance Deployment, Project Specification, Decision Record, Runbook, Strategy Document, Research Analysis, Infrastructure Component, Proposal, Meeting Note.

Path convention: {type-plural}/{slug}.md (e.g. clients/neh.md, decisions/sqitch-migrations.md, runbooks/deploy-new-version.md).`,
	}
}
