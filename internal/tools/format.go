package tools

import (
	"fmt"
	"strings"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// linkRules is shared by the descriptions of tools that write concept content.
const linkRules = `Links: reference other concepts with markdown links using absolute bundle paths, e.g. [Client Profile](/clients/acme.md). Relative links and links to a concept's former path are normalized automatically. A write that adds a link to a concept that doesn't exist is rejected with suggestions, so create concepts in dependency order and add back-links afterwards with kb_update. External links with a scheme (https://, mailto:, …) are free-form and never checked. Bare paths in text or in code spans are not links.`

// typeTable lists the registered folders and the type each one decides.
func typeTable() string {
	var sb strings.Builder
	sb.WriteString("Folders and their types (the folder decides the type):")
	for _, t := range bundle.Types {
		fmt.Fprintf(&sb, "\n- %s/ — %s: %s", t.Folder, t.Name, t.Holds)
	}
	return sb.String()
}

// cleanPath accepts concept paths written as link targets ("/clients/acme.md").
func cleanPath(p string) string {
	return strings.TrimPrefix(strings.TrimSpace(p), "/")
}

// errorResult builds a tool error result.
func errorResult(format string, args ...any) *mcp.CallToolResult {
	result := &mcp.CallToolResult{}
	result.SetError(fmt.Errorf(format, args...))
	return result
}

// textResult builds a successful text tool result.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// notFoundResult reports a missing concept, with suggestions when there are any.
func notFoundResult(deps *Deps, action, path string, err error) *mcp.CallToolResult {
	return errorResult("%s failed: %v%s", action, err, bundle.DidYouMean(deps.Bundle.Suggest(path)))
}

// movedHeader tells the caller a read followed an alias.
func movedHeader(requested, current string) string {
	return fmt.Sprintf("[moved: %s → %s — use the new path]\n", requested, current)
}

// formatLinkReport renders link normalizations and warnings for a write result.
func formatLinkReport(r *bundle.LinkReport) string {
	if r == nil || (len(r.Normalized) == 0 && len(r.Warnings) == 0) {
		return ""
	}
	var sb strings.Builder
	if len(r.Normalized) > 0 {
		sb.WriteString("\n\nNormalized links:")
		for _, n := range r.Normalized {
			fmt.Fprintf(&sb, "\n- %s → %s", n.From, n.To)
		}
	}
	if len(r.Warnings) > 0 {
		sb.WriteString("\n\nLink warnings:")
		for _, w := range r.Warnings {
			sb.WriteString("\n- " + w)
		}
	}
	return sb.String()
}
