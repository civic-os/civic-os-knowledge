package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type LinksInput struct {
	Path string `json:"path,omitempty" jsonschema:"Concept to inspect. Omit for a knowledgebase-wide link audit."`
}

func LinksHandler(deps *Deps) func(context.Context, *mcp.CallToolRequest, *LinksInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input *LinksInput) (*mcp.CallToolResult, any, error) {
		path := cleanPath(input.Path)
		if path == "" {
			audit, err := deps.Bundle.AuditLinks()
			if err != nil {
				return errorResult("link audit failed: %w", err), nil, nil
			}
			concepts, err := deps.Bundle.List()
			if err != nil {
				return errorResult("link audit failed: %w", err), nil, nil
			}
			var typeProblems []string
			for _, c := range concepts {
				if p := bundle.TypeProblem(c); p != "" {
					typeProblems = append(typeProblems, p)
				}
			}
			return textResult(formatLinkAudit(audit, typeProblems)), nil, nil
		}

		info, err := deps.Bundle.LinksOf(path)
		if err != nil {
			return notFoundResult(deps, "links", path, err), nil, nil
		}
		return textResult(formatLinkInfo(info)), nil, nil
	}
}

func formatLinkInfo(info *bundle.LinkInfo) string {
	var sb strings.Builder
	if info.ResolvedFrom != "" {
		sb.WriteString(movedHeader(info.ResolvedFrom, info.Path))
	}
	fmt.Fprintf(&sb, "Links for %s\n", info.Path)

	fmt.Fprintf(&sb, "\nOutbound concept links (%d):\n", len(info.Outbound))
	for _, t := range info.Outbound {
		line := "- " + t.Path + countSuffix(t.Count)
		if t.Broken {
			line += " — BROKEN" + bundle.DidYouMean(t.Suggestions)
		}
		sb.WriteString(line + "\n")
	}
	fmt.Fprintf(&sb, "External links: %d\n", info.External)

	fmt.Fprintf(&sb, "\nInbound links (%d):\n", len(info.Inbound))
	for _, s := range info.Inbound {
		sb.WriteString("- " + s.Path + countSuffix(s.Count) + "\n")
	}

	if len(info.Mentions) > 0 {
		fmt.Fprintf(&sb, "\nNamed without a markdown link (%d) — kb_move won't rewrite these:\n", len(info.Mentions))
		for _, s := range info.Mentions {
			line := "- " + s.Path + countSuffix(s.Count)
			if s.InCode {
				line += " (in code)"
			}
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

func formatLinkAudit(a *bundle.LinkAudit, typeProblems []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Link audit: %d concepts, %d concept links, %d external links.\n", a.Concepts, a.Links, a.External)

	fmt.Fprintf(&sb, "\nBroken links (%d):\n", len(a.Broken))
	for _, p := range a.Broken {
		sb.WriteString("- " + p.Source + " → " + p.Target + bundle.DidYouMean(p.Suggestions) + "\n")
	}

	fmt.Fprintf(&sb, "\nNon-canonical links (%d):\n", len(a.NonCanonical))
	for _, p := range a.NonCanonical {
		sb.WriteString("- " + p.Source + ": " + p.Target + " → " + p.Canonical + "\n")
	}

	fmt.Fprintf(&sb, "\nPlain-text mentions of concept paths (%d):\n", len(a.Mentions))
	for _, p := range a.Mentions {
		sb.WriteString("- " + p.Source + " names " + p.Target + "\n")
	}

	fmt.Fprintf(&sb, "\nFolder/type mismatches (%d):\n", len(typeProblems))
	for _, p := range typeProblems {
		sb.WriteString("- " + p + "\n")
	}
	return sb.String()
}

func countSuffix(n int) string {
	if n > 1 {
		return fmt.Sprintf(" ×%d", n)
	}
	return ""
}

func LinksTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "kb_links",
		Description: "Show a concept's outbound concept links (flagging broken ones, with suggestions), the concepts that link to it, and concepts that name its path without a markdown link. Use it before kb_move to see what will be rewritten. Omit path for a knowledgebase-wide audit of broken links, non-canonical links, plain-text mentions of concept paths, and concepts whose type doesn't match their folder.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}
}
