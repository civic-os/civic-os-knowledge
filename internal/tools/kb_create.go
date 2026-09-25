package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateInput struct {
	Path        string   `json:"path" jsonschema:"Relative file path for the new concept (e.g. clients/newclient.md)"`
	Type        string   `json:"type" jsonschema:"Concept type: Client Profile, Instance Deployment, Project Specification, Decision Record, Runbook, Strategy Document, Research Analysis, Infrastructure Component, Proposal, Meeting Note, Prospect, Competitive Analysis, Marketing Document"`
	Title       string   `json:"title" jsonschema:"Human-readable title"`
	Description string   `json:"description,omitempty" jsonschema:"One-sentence description"`
	Resource    string   `json:"resource,omitempty" jsonschema:"External resource URL"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Tags for categorization"`
	Body        string   `json:"body,omitempty" jsonschema:"Markdown body content"`
	Status      string   `json:"status,omitempty" jsonschema:"Concept lifecycle status: draft, stable, or deprecated. Defaults to stable (omitted from YAML)."`
}

func CreateHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *CreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *CreateInput) (*mcp.CallToolResult, any, error) {
		// Default empty status to stable; validate
		status := input.Status
		if status == "" {
			status = bundle.StatusStable
		}
		if !bundle.ValidStatus(status) {
			return errorResult("invalid status %q: must be draft, stable, or deprecated", input.Status), nil, nil
		}

		path := cleanPath(input.Path)
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
			Path: path,
		}
		// Only persist non-stable status in frontmatter
		if status != bundle.StatusStable {
			c.Meta.Status = status
		}

		report, err := deps.Bundle.Create(c)
		if err != nil {
			var broken *bundle.BrokenLinksError
			if errors.As(err, &broken) {
				return errorResult("create rejected: %w", err), nil, nil
			}
			return errorResult("create failed: %w", err), nil, nil
		}

		deps.Index.Add(c)
		deps.onWrite(path)

		return textResult(fmt.Sprintf("Created concept: %s (version: 1)", path) + formatLinkReport(report)), nil, nil
	}
}

func CreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "kb_create",
		Description: `Create a new knowledge concept. Each concept should capture one idea, decision, or artifact — prefer creating a new linked concept over expanding an existing one.

Concept types: Client Profile, Instance Deployment, Project Specification, Decision Record, Runbook, Strategy Document, Research Analysis, Infrastructure Component, Proposal, Meeting Note, Prospect, Competitive Analysis, Marketing Document.

Path convention: {type-plural}/{slug}.md (e.g. clients/neh.md, decisions/sqitch-migrations.md, runbooks/deploy-new-version.md, prospects/city-of-example.md, competitive-analysis/vendor-name.md, marketing/brand-voice.md).

Status: draft | stable | deprecated. Defaults to stable (omitted from YAML). Use draft for work-in-progress concepts, deprecated for concepts that should no longer be referenced.

` + linkRules,
	}
}
