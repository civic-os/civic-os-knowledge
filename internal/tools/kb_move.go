package tools

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MoveItemInput struct {
	Path    string `json:"path" jsonschema:"Current relative path of the concept (e.g. strategy/brand-voice.md)"`
	NewPath string `json:"new_path" jsonschema:"New relative path (e.g. marketing/brand-voice.md)"`
	Version int    `json:"version,omitempty" jsonschema:"Expected version from kb_read. If provided, the batch is rejected when the concept changed since your read."`
}

type MoveInput struct {
	Moves []MoveItemInput `json:"moves" jsonschema:"One or more moves, validated together and applied all-or-nothing"`
}

func MoveHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *MoveInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *MoveInput) (*mcp.CallToolResult, any, error) {
		items := make([]bundle.MoveItem, len(input.Moves))
		for i, m := range input.Moves {
			items[i] = bundle.MoveItem{Path: cleanPath(m.Path), NewPath: cleanPath(m.NewPath), Version: m.Version}
		}

		res, err := deps.Bundle.MoveBatch(items)
		if err != nil {
			var moved *bundle.MovedError
			switch {
			case errors.Is(err, bundle.ErrConflict):
				return errorResult("Conflict: %v. Nothing was moved; re-read with kb_read and retry.", err), nil, nil
			case errors.As(err, &moved):
				return errorResult("%v. Nothing was moved.", err), nil, nil
			}
			return errorResult("move failed: %v. Nothing was moved.", err), nil, nil
		}

		for _, m := range res.Moved {
			deps.Index.Remove(m.From)
			deps.Index.Add(m.Concept)
		}
		for _, c := range res.Relinked {
			deps.Index.Add(c)
		}

		// Writes before deletes, so an interrupted S3 sync leaves duplicates, never gaps.
		for _, p := range res.Written {
			deps.onWrite(p)
		}
		for _, p := range res.VersionsWritten {
			deps.onSnapshot(p)
		}
		for _, p := range res.VersionsRemoved {
			deps.onSnapshotDelete(p)
		}
		for _, p := range res.Removed {
			deps.onDelete(p)
		}

		return textResult(formatMoveResult(res)), nil, nil
	}
}

func formatMoveResult(res *bundle.MoveResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Moved %d concept(s):\n", len(res.Moved))
	for _, m := range res.Moved {
		fmt.Fprintf(&sb, "- %s → %s (version %d → %d, %d snapshot(s) carried)\n",
			m.From, m.Concept.Path, m.Concept.Version-1, m.Concept.Version, m.Carried)
	}

	if len(res.Relinked) > 0 {
		fmt.Fprintf(&sb, "\nUpdated %d other concept(s) (links rewritten, stale aliases removed):\n", len(res.Relinked))
		for _, c := range res.Relinked {
			fmt.Fprintf(&sb, "- %s (version %d)\n", c.Path, c.Version)
		}
	}

	if len(res.Mentions) > 0 {
		olds := make([]string, 0, len(res.Mentions))
		for old := range res.Mentions {
			olds = append(olds, old)
		}
		sort.Strings(olds)
		sb.WriteString("\nOld paths still named outside markdown links (not rewritten; edit by hand if they're references):\n")
		for _, old := range olds {
			fmt.Fprintf(&sb, "- %s: %s\n", old, strings.Join(res.Mentions[old], ", "))
		}
	}
	return sb.String()
}

func MoveTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "kb_move",
		Description: `Move (rename) one or more concepts. Use it to reorganize the knowledgebase, e.g. moving marketing material from strategy/ to marketing/.

- History follows the concept: version snapshots move to the new path and numbering continues.
- The old path is recorded under "aliases" in the concept's frontmatter, so reads of the old path still find it.
- Markdown links to a moved concept are rewritten across the knowledgebase. Every rewritten concept, and the moved concept itself, gets one new version.
- Bare paths outside markdown links are reported but not rewritten.

Batch related moves in a single call so each affected concept is rewritten once. A batch is validated in full and applied all-or-nothing. Chains (a→b plus b→c) and swaps aren't allowed in one batch.

Run kb_links on a concept first to see what links to it. Moving doesn't change the concept's type; use kb_update for that.`,
	}
}
