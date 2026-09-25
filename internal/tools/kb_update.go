package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type UpdateInput struct {
	Path        string   `json:"path" jsonschema:"Relative file path of the concept to update"`
	Type        string   `json:"type,omitempty" jsonschema:"Concept type. Only the type of the concept's folder is accepted (to fix older concepts); use kb_move to recategorize"`
	Title       string   `json:"title,omitempty" jsonschema:"Updated title"`
	Description string   `json:"description,omitempty" jsonschema:"Updated description"`
	Resource    string   `json:"resource,omitempty" jsonschema:"Updated resource URL"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Updated tags"`
	Body        string   `json:"body,omitempty" jsonschema:"Updated markdown body content"`
	Status      string   `json:"status,omitempty" jsonschema:"Lifecycle status: draft, stable, or deprecated"`
	Version     int      `json:"version,omitempty" jsonschema:"Expected version from kb_read. If provided, update is rejected when another session has written since your read."`
}

func UpdateHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *UpdateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *UpdateInput) (*mcp.CallToolResult, any, error) {
		path := cleanPath(input.Path)

		// Read existing to merge fields
		existing, err := deps.Bundle.Read(path)
		if err != nil {
			return notFoundResult(deps, "update", path, fmt.Errorf("concept not found: %w", err)), nil, nil
		}
		// Reads follow aliases; writes don't, so the caller re-reads the current path.
		if existing.ResolvedFrom != "" {
			return errorResult("%v", &bundle.MovedError{Path: path, Current: existing.Path}), nil, nil
		}

		// Validate status if provided
		if input.Status != "" && !bundle.ValidStatus(input.Status) {
			return errorResult("invalid status %q: must be draft, stable, or deprecated", input.Status), nil, nil
		}

		// The folder decides the type: a type change must match it.
		if input.Type != "" {
			conceptType, err := bundle.ResolveType(path, input.Type)
			if err != nil {
				return errorResult("update rejected: %v. To change a concept's type, move it into that type's folder with kb_move.", err), nil, nil
			}
			existing.Meta.Type = conceptType
		}

		// Merge: only update fields that are provided
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
		if input.Status != "" {
			existing.Meta.Status = input.Status
		}
		existing.Meta.Timestamp = bundle.NowTimestamp()

		report, err := deps.Bundle.Update(existing, input.Version)
		if err != nil {
			var broken *bundle.BrokenLinksError
			var moved *bundle.MovedError
			switch {
			case errors.Is(err, bundle.ErrConflict):
				// Re-read to get current version for the error message
				current, readErr := deps.Bundle.Read(path)
				currentVer := 0
				if readErr == nil {
					currentVer = current.Version
				}
				return errorResult("Conflict: concept modified since your read (current version: %d). Re-read with kb_read.", currentVer), nil, nil
			case errors.As(err, &broken):
				return errorResult("update rejected: %w", err), nil, nil
			case errors.As(err, &moved):
				return errorResult("%w", err), nil, nil
			}
			return errorResult("update failed: %w", err), nil, nil
		}

		deps.Index.Add(existing)
		deps.onWrite(path)
		deps.onSnapshot(bundle.SnapshotPath(path, existing.Version-1))

		text := fmt.Sprintf("Updated concept: %s (version: %d)", path, existing.Version) + formatLinkReport(report)
		if problem := bundle.TypeProblem(existing); problem != "" {
			text += "\n\nType warning: " + problem + "."
		}
		return textResult(text), nil, nil
	}
}

func UpdateTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "kb_update",
		Description: `Update an existing knowledge concept. Only specified fields are changed; others are preserved. A version snapshot is created before updating.

Use updates for corrections, status changes, and metadata fixes. For substantial new knowledge, prefer creating a new linked concept rather than appending to an existing one — this keeps concepts focused and the knowledge graph navigable.

A concept's folder decides its type. To recategorize a concept, or to rename it, use kb_move; kb_update only accepts the type of the concept's current folder (useful for fixing older concepts).

Pass the version number from kb_read to enable optimistic concurrency control. If another session updated the concept since your read, the update is rejected with a conflict error — re-read and retry.

Status lifecycle: set status to "deprecated" to mark a concept as superseded without deleting it. Use "draft" for work-in-progress.

` + linkRules,
	}
}
