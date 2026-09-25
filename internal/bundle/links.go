package bundle

import (
	"path"
	"regexp"
	"strings"
)

// Link model
//
// A concept link is a markdown link [text](target) whose target has no URL
// scheme and ends in .md, optionally followed by #anchor. Its canonical form
// is an absolute bundle path: /dir/slug.md. Targets with a scheme (https://,
// chrome://, mailto:) are external and are never checked or rewritten.
// Anything inside fenced code blocks or inline code spans is not a link, and
// neither is a bare path outside link syntax (a "mention").

var (
	linkRe   = regexp.MustCompile(`\]\(([^)\s]+)`)
	schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// LinkKind classifies a markdown link target.
type LinkKind int

const (
	LinkIgnored  LinkKind = iota // e.g. #section, /clients/, image.png
	LinkConcept                  // bundle concept, e.g. /clients/acme.md
	LinkExternal                 // has a URL scheme, e.g. https://example.com
)

// Link is a markdown link found in a text.
type Link struct {
	Target   string // raw target as written
	Kind     LinkKind
	Resolved string // bundle-relative path for concept links; "" if it escapes the bundle
	Anchor   string // "#..." suffix of a concept link, if any
	Start    int    // byte offset of Target in the text
	End      int
}

// Mention is an occurrence of a concept path outside link syntax.
type Mention struct {
	Path   string // bundle-relative concept path mentioned
	InCode bool   // inside a code span or fenced block
}

// Resolve returns the bundle-relative path a concept link target points to.
// Absolute targets resolve from the bundle root; relative targets resolve
// against the source concept's directory. Returns "" if the target escapes
// the bundle.
func Resolve(source, target string) string {
	t, _ := splitAnchor(target)
	var p string
	if strings.HasPrefix(t, "/") {
		p = strings.TrimPrefix(path.Clean(t), "/")
	} else {
		p = path.Join(path.Dir(source), t)
	}
	if p == "" || p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	return p
}

// CanonicalTarget returns the canonical link target for a concept path.
func CanonicalTarget(conceptPath, anchor string) string {
	return "/" + conceptPath + anchor
}

// FindLinks returns the markdown links in text, skipping code.
func FindLinks(text, source string) []Link {
	masked := codeMask(text)
	var links []Link
	for _, m := range linkRe.FindAllSubmatchIndex(masked, -1) {
		l := Link{Target: text[m[2]:m[3]], Start: m[2], End: m[3]}
		l.Kind = classify(l.Target)
		if l.Kind == LinkConcept {
			l.Resolved = Resolve(source, l.Target)
			_, l.Anchor = splitAnchor(l.Target)
		}
		links = append(links, l)
	}
	return links
}

// RewriteLinks replaces concept link targets in text. fn is called for each
// concept link and returns the replacement target and true to rewrite it.
// Link text and everything outside rewritten targets is left byte-for-byte.
// Returns the new text and the number of targets rewritten.
func RewriteLinks(text, source string, fn func(Link) (string, bool)) (string, int) {
	var sb strings.Builder
	last, n := 0, 0
	for _, l := range FindLinks(text, source) {
		if l.Kind != LinkConcept {
			continue
		}
		target, ok := fn(l)
		if !ok || target == l.Target {
			continue
		}
		sb.WriteString(text[last:l.Start])
		sb.WriteString(target)
		last = l.End
		n++
	}
	if n == 0 {
		return text, 0
	}
	sb.WriteString(text[last:])
	return sb.String(), n
}

// ConceptLinks returns the unique resolved concept paths linked from a
// concept's title, description and body, in order of first appearance.
func ConceptLinks(c *Concept) []string {
	seen := make(map[string]bool)
	var out []string
	for _, text := range markdownFields(c) {
		for _, l := range FindLinks(text, c.Path) {
			if l.Kind == LinkConcept && l.Resolved != "" && !seen[l.Resolved] {
				seen[l.Resolved] = true
				out = append(out, l.Resolved)
			}
		}
	}
	return out
}

// FindMentions returns occurrences of the given concept paths in text that
// are not link targets, with or without a leading slash.
func FindMentions(text, source string, paths []string) []Mention {
	links := FindLinks(text, source)
	var masked []byte
	var out []Mention
	for _, p := range paths {
		for from := 0; ; {
			i := strings.Index(text[from:], p)
			if i < 0 {
				break
			}
			i += from
			from = i + len(p)
			if !mentionBoundaryBefore(text, i) || !mentionBoundaryAfter(text, i+len(p)) || insideLink(links, i) {
				continue
			}
			if masked == nil {
				masked = codeMask(text)
			}
			out = append(out, Mention{Path: p, InCode: masked[i] != text[i]})
		}
	}
	return out
}

// markdownFields returns the concept fields that may contain markdown links.
func markdownFields(c *Concept) []string {
	return []string{c.Meta.Title, c.Meta.Description, c.Body}
}

func classify(target string) LinkKind {
	if schemeRe.MatchString(target) || strings.HasPrefix(target, "//") {
		return LinkExternal
	}
	if p, _ := splitAnchor(target); strings.HasSuffix(p, ".md") {
		return LinkConcept
	}
	return LinkIgnored
}

func splitAnchor(target string) (string, string) {
	if i := strings.IndexByte(target, '#'); i >= 0 {
		return target[:i], target[i:]
	}
	return target, ""
}

func insideLink(links []Link, i int) bool {
	for _, l := range links {
		if i >= l.Start && i < l.End {
			return true
		}
	}
	return false
}

func isPathByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == '.' || c == '/'
}

// mentionBoundaryBefore rejects matches that are the tail of a longer path,
// e.g. "strategy/foo.md" inside "marketing/strategy/foo.md".
func mentionBoundaryBefore(text string, i int) bool {
	if i == 0 {
		return true
	}
	if text[i-1] == '/' {
		return i == 1 || !isPathByte(text[i-2])
	}
	return !isPathByte(text[i-1])
}

func mentionBoundaryAfter(text string, j int) bool {
	if j == len(text) {
		return true
	}
	c := text[j]
	return c == '.' || c == '#' || !isPathByte(c)
}

// codeMask returns a copy of text with fenced code blocks and inline code
// spans blanked to spaces (newlines kept), preserving byte offsets.
func codeMask(text string) []byte {
	out := []byte(text)
	blank := func(from, to int) {
		for k := from; k < to; k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}

	inFence := false
	fence := ""
	for start := 0; start < len(out); {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(out)
		} else {
			end += start
		}
		line := text[start:end]
		trimmed := strings.TrimLeft(line, " ")
		isFence := len(line)-len(trimmed) <= 3 &&
			(strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"))
		switch {
		case inFence:
			blank(start, end)
			if isFence && strings.HasPrefix(trimmed, fence) {
				inFence = false
			}
		case isFence:
			inFence, fence = true, trimmed[:3]
			blank(start, end)
		}
		start = end + 1
	}

	backtickRun := func(i int) int {
		n := 0
		for i+n < len(out) && out[i+n] == '`' {
			n++
		}
		return n
	}
	for i := 0; i < len(out); {
		if out[i] != '`' {
			i++
			continue
		}
		n := backtickRun(i)
		closeAt := -1
		for j := i + n; j < len(out); {
			if out[j] != '`' {
				j++
				continue
			}
			m := backtickRun(j)
			if m == n {
				closeAt = j
				break
			}
			j += m
		}
		if closeAt < 0 {
			i += n
			continue
		}
		blank(i, closeAt+n)
		i = closeAt + n
	}
	return out
}

// suggest returns up to three existing concept paths a broken link target may
// have meant. paths must be sorted; aliases maps alias path to its holder.
func suggest(raw, resolved string, paths []string, exists map[string]bool, aliases map[string]string) []string {
	var out []string
	add := func(p string) {
		if p == "" || len(out) >= 3 {
			return
		}
		for _, o := range out {
			if o == p {
				return
			}
		}
		out = append(out, p)
	}

	if h, ok := aliases[resolved]; ok {
		add(h)
	}
	if t, _ := splitAnchor(raw); !strings.HasPrefix(t, "/") {
		if r := strings.TrimPrefix(path.Clean("/"+t), "/"); exists[r] {
			add(r)
		}
	}
	base := path.Base(resolved)
	if resolved == "" {
		t, _ := splitAnchor(raw)
		base = path.Base(t)
	}
	for _, p := range paths {
		if path.Base(p) == base {
			add(p)
		}
	}
	stem := strings.TrimSuffix(base, ".md")
	for _, p := range paths {
		ps := strings.TrimSuffix(path.Base(p), ".md")
		if len(stem) >= 3 && len(ps) >= 3 && (strings.HasPrefix(ps, stem) || strings.HasPrefix(stem, ps)) {
			add(p)
		}
	}
	return out
}
