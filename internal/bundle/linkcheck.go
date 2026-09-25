package bundle

import (
	"fmt"
	"strings"
)

// LinkReport describes what link checks did during a write.
type LinkReport struct {
	Normalized []LinkChange // concept link targets rewritten to canonical form
	Warnings   []string
}

// LinkChange is a link target rewritten during normalization.
type LinkChange struct {
	From, To string
}

// BrokenLink is a concept link whose target doesn't exist.
type BrokenLink struct {
	Target      string   // as written
	Suggestions []string // existing concept paths it may have meant
}

// BrokenLinksError is returned when a write would introduce links to
// concepts that don't exist. Nothing is written.
type BrokenLinksError struct {
	Links []BrokenLink
}

func (e *BrokenLinksError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d link(s) point to concepts that don't exist:", len(e.Links))
	for _, l := range e.Links {
		sb.WriteString("\n- " + l.Target + DidYouMean(l.Suggestions))
	}
	sb.WriteString("\nFix the link, create the target concept first, or use a full URL for documents outside the knowledgebase.")
	return sb.String()
}

// checkLinksUnlocked normalizes c's concept links in place and enforces the
// link rules: links introduced by this write must resolve to existing
// concepts; broken links already present in prev only warn. Plain-text
// mentions of concept paths also warn.
func (b *Bundle) checkLinksUnlocked(c *Concept, prev *Concept) (*LinkReport, error) {
	paths, err := b.pathsUnlocked()
	if err != nil {
		return nil, err
	}
	exists := pathSet(paths)
	valid := func(p string) bool { return p != "" && (p == c.Path || exists[p]) }

	var aliases map[string]string
	aliasMap := func() map[string]string {
		if aliases == nil {
			if aliases, _ = b.aliasMapUnlocked(); aliases == nil {
				aliases = map[string]string{}
			}
		}
		return aliases
	}

	report := &LinkReport{}

	// Normalize: canonical /dir/slug.md form, following aliases to current paths.
	for _, field := range []*string{&c.Meta.Title, &c.Meta.Description, &c.Body} {
		*field, _ = RewriteLinks(*field, c.Path, func(l Link) (string, bool) {
			target := l.Resolved
			if !valid(target) {
				holder, ok := aliasMap()[target]
				if !ok {
					return "", false
				}
				target = holder
			}
			canonical := CanonicalTarget(target, l.Anchor)
			if canonical != l.Target {
				report.Normalized = append(report.Normalized, LinkChange{From: l.Target, To: canonical})
			}
			return canonical, true
		})
	}

	// Broken links: new ones reject the write, pre-existing ones warn.
	prevBroken := make(map[string]bool)
	if prev != nil {
		for _, text := range markdownFields(prev) {
			for _, l := range FindLinks(text, prev.Path) {
				if l.Kind == LinkConcept {
					prevBroken[brokenKey(l)] = true
				}
			}
		}
	}
	var broken []BrokenLink
	seen := make(map[string]bool)
	for _, text := range markdownFields(c) {
		for _, l := range FindLinks(text, c.Path) {
			if l.Kind != LinkConcept || valid(l.Resolved) || seen[brokenKey(l)] {
				continue
			}
			seen[brokenKey(l)] = true
			suggestions := suggest(l.Target, l.Resolved, paths, exists, aliasMap())
			if prevBroken[brokenKey(l)] {
				report.Warnings = append(report.Warnings, "pre-existing broken link "+l.Target+DidYouMean(suggestions))
				continue
			}
			broken = append(broken, BrokenLink{Target: l.Target, Suggestions: suggestions})
		}
	}
	if len(broken) > 0 {
		return report, &BrokenLinksError{Links: broken}
	}

	// Plain-text mentions of other concepts should be markdown links.
	others := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != c.Path {
			others = append(others, p)
		}
	}
	mentioned := make(map[string]bool)
	for _, text := range []string{c.Meta.Title, c.Meta.Description, c.Meta.Resource, c.Body} {
		for _, m := range FindMentions(text, c.Path, others) {
			if m.InCode || mentioned[m.Path] {
				continue
			}
			mentioned[m.Path] = true
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"plain-text mention of %s; if it's a reference, make it a markdown link: [title](/%s)", m.Path, m.Path))
		}
	}
	return report, nil
}

// brokenKey identifies a link target for comparing broken links across versions.
func brokenKey(l Link) string {
	if l.Resolved != "" {
		return l.Resolved
	}
	return l.Target
}

// DidYouMean formats suggestions as " (did you mean a, b?)", or "" if there are none.
func DidYouMean(suggestions []string) string {
	if len(suggestions) == 0 {
		return ""
	}
	return " (did you mean " + strings.Join(suggestions, ", ") + "?)"
}

// LinkTarget is an outbound concept link from one concept.
type LinkTarget struct {
	Path        string // resolved target path, or the raw target if it escapes the bundle
	Count       int
	Broken      bool
	Suggestions []string
}

// LinkSource is a concept that links to, or mentions, another concept.
type LinkSource struct {
	Path   string
	Count  int
	InCode bool // for mentions: every occurrence is inside code
}

// LinkInfo describes one concept's place in the link graph.
type LinkInfo struct {
	Path         string
	ResolvedFrom string
	Outbound     []LinkTarget
	Inbound      []LinkSource
	Mentions     []LinkSource // concepts naming this path outside link syntax
	External     int          // links with a URL scheme
}

// LinksOf returns the outbound links, inbound links and mentions of a
// concept, following aliases.
func (b *Bundle) LinksOf(p string) (*LinkInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	current, viaAlias := b.resolveUnlocked(p)
	if !b.exists(current) {
		return nil, fmt.Errorf("concept not found: %s", p)
	}
	concepts, err := b.listUnlocked()
	if err != nil {
		return nil, err
	}
	paths, err := b.pathsUnlocked()
	if err != nil {
		return nil, err
	}
	exists := pathSet(paths)
	aliases, _ := b.aliasMapUnlocked()

	info := &LinkInfo{Path: current}
	if viaAlias {
		info.ResolvedFrom = p
	}
	outIdx := make(map[string]int)
	for _, c := range concepts {
		if c.Path == current {
			for _, text := range markdownFields(c) {
				for _, l := range FindLinks(text, c.Path) {
					switch l.Kind {
					case LinkExternal:
						info.External++
					case LinkConcept:
						key := brokenKey(l)
						if i, ok := outIdx[key]; ok {
							info.Outbound[i].Count++
							continue
						}
						t := LinkTarget{Path: key, Count: 1}
						if l.Resolved == "" || (l.Resolved != c.Path && !exists[l.Resolved]) {
							t.Broken = true
							t.Suggestions = suggest(l.Target, l.Resolved, paths, exists, aliases)
						}
						outIdx[key] = len(info.Outbound)
						info.Outbound = append(info.Outbound, t)
					}
				}
			}
			continue
		}

		in := 0
		for _, text := range markdownFields(c) {
			for _, l := range FindLinks(text, c.Path) {
				if l.Kind == LinkConcept && l.Resolved == current {
					in++
				}
			}
		}
		if in > 0 {
			info.Inbound = append(info.Inbound, LinkSource{Path: c.Path, Count: in})
		}

		var mentions []Mention
		for _, text := range []string{c.Meta.Title, c.Meta.Description, c.Meta.Resource, c.Body} {
			mentions = append(mentions, FindMentions(text, c.Path, []string{current})...)
		}
		if len(mentions) > 0 {
			src := LinkSource{Path: c.Path, Count: len(mentions), InCode: true}
			for _, m := range mentions {
				src.InCode = src.InCode && m.InCode
			}
			info.Mentions = append(info.Mentions, src)
		}
	}
	return info, nil
}

// LinkProblem is a link or mention flagged by AuditLinks.
type LinkProblem struct {
	Source      string
	Target      string   // as written, or the mentioned path
	Canonical   string   // non-canonical links: the canonical form
	Suggestions []string // broken links: what it may have meant
}

// LinkAudit is a bundle-wide link report.
type LinkAudit struct {
	Concepts     int
	Links        int // concept links
	External     int
	Broken       []LinkProblem
	NonCanonical []LinkProblem
	Mentions     []LinkProblem // plain-text mentions of concept paths
}

// AuditLinks checks every concept's links: broken targets, links not in
// canonical form, and plain-text mentions of concept paths.
func (b *Bundle) AuditLinks() (*LinkAudit, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	concepts, err := b.listUnlocked()
	if err != nil {
		return nil, err
	}
	paths, err := b.pathsUnlocked()
	if err != nil {
		return nil, err
	}
	exists := pathSet(paths)
	aliases, _ := b.aliasMapUnlocked()

	audit := &LinkAudit{Concepts: len(concepts)}
	for _, c := range concepts {
		for _, text := range markdownFields(c) {
			for _, l := range FindLinks(text, c.Path) {
				switch l.Kind {
				case LinkExternal:
					audit.External++
					continue
				case LinkIgnored:
					continue
				}
				audit.Links++
				if l.Resolved == "" || (l.Resolved != c.Path && !exists[l.Resolved]) {
					audit.Broken = append(audit.Broken, LinkProblem{
						Source: c.Path, Target: l.Target,
						Suggestions: suggest(l.Target, l.Resolved, paths, exists, aliases),
					})
				} else if canonical := CanonicalTarget(l.Resolved, l.Anchor); canonical != l.Target {
					audit.NonCanonical = append(audit.NonCanonical, LinkProblem{Source: c.Path, Target: l.Target, Canonical: canonical})
				}
			}
		}

		others := make([]string, 0, len(paths))
		for _, p := range paths {
			if p != c.Path {
				others = append(others, p)
			}
		}
		mentioned := make(map[string]bool)
		for _, text := range []string{c.Meta.Title, c.Meta.Description, c.Meta.Resource, c.Body} {
			for _, m := range FindMentions(text, c.Path, others) {
				if !m.InCode && !mentioned[m.Path] {
					mentioned[m.Path] = true
					audit.Mentions = append(audit.Mentions, LinkProblem{Source: c.Path, Target: m.Path})
				}
			}
		}
	}
	return audit, nil
}
