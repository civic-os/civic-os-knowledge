package bundle

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		source, target, want string
	}{
		{"clients/a.md", "/instances/b.md", "instances/b.md"},
		{"clients/a.md", "b.md", "clients/b.md"},
		{"a.md", "b.md", "b.md"},
		{"deep/nested/a.md", "/top.md", "top.md"},
		{"clients/a.md", "../instances/b.md", "instances/b.md"},
		{"clients/a.md", "/instances/b.md#status", "instances/b.md"},
		{"clients/a.md", "/a/../b.md", "b.md"},
		{"clients/a.md", "../../escape.md", ""},
		{"clients/a.md", "/../escape.md", "escape.md"},
	}
	for _, tt := range tests {
		if got := Resolve(tt.source, tt.target); got != tt.want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", tt.source, tt.target, got, tt.want)
		}
	}
}

func TestFindLinksClassification(t *testing.T) {
	text := "[a](/clients/a.md) [b](b.md#x) [web](https://example.com/SPEC.md) " +
		"[chrome](chrome://settings) [mail](mailto:x@example.com) [proto](//cdn.example.com/x.md) " +
		"[sec](#heading) [dir](/clients/) [img](diagram.png)"
	var kinds []LinkKind
	for _, l := range FindLinks(text, "notes/n.md") {
		kinds = append(kinds, l.Kind)
	}
	want := []LinkKind{
		LinkConcept, LinkConcept, LinkExternal,
		LinkExternal, LinkExternal, LinkExternal,
		LinkIgnored, LinkIgnored, LinkIgnored,
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}

	links := FindLinks("[b](b.md#x)", "notes/n.md")
	if links[0].Resolved != "notes/b.md" || links[0].Anchor != "#x" {
		t.Errorf("resolved %q anchor %q", links[0].Resolved, links[0].Anchor)
	}
}

func TestFindLinksSkipsCode(t *testing.T) {
	text := "Real [a](/a.md).\n\n" +
		"Inline `[b](/b.md)` and ``[c](/c.md) with ` inside``.\n\n" +
		"```markdown\n[d](/d.md)\n```\n\n" +
		"~~~\n[e](/e.md)\n~~~\n\n" +
		"After [f](/f.md)."
	var got []string
	for _, l := range FindLinks(text, "x.md") {
		got = append(got, l.Resolved)
	}
	if want := []string{"a.md", "f.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("links = %v, want %v", got, want)
	}
}

func TestFindLinksUnclosedBacktick(t *testing.T) {
	// A lone backtick doesn't open a code span, so the link still counts.
	links := FindLinks("It's 5` tall, see [a](/a.md).", "x.md")
	if len(links) != 1 || links[0].Resolved != "a.md" {
		t.Errorf("links = %+v", links)
	}
}

func TestRewriteLinks(t *testing.T) {
	text := `### Pricing (see [the record](/strategy/foo.md))

| Contact | Notes |
|---------|-------|
| Ann | [Foo](/strategy/foo.md) |

> **Superseded** by [Foo](/strategy/foo.md#details).

**[Foo, bold](foo.md)** and [Other](/strategy/other.md) and [web](https://x.test/strategy/foo.md).

Backticked ` + "`/strategy/foo.md`" + ` and bare strategy/foo.md stay.`

	out, n := RewriteLinks(text, "strategy/bar.md", func(l Link) (string, bool) {
		if l.Resolved == "strategy/foo.md" {
			return CanonicalTarget("marketing/foo.md", l.Anchor), true
		}
		return "", false
	})
	if n != 4 {
		t.Errorf("rewrote %d links, want 4", n)
	}
	for _, want := range []string{
		"[the record](/marketing/foo.md))",
		"| Ann | [Foo](/marketing/foo.md) |",
		"[Foo](/marketing/foo.md#details)",
		"**[Foo, bold](/marketing/foo.md)**",
		"[Other](/strategy/other.md)",
		"[web](https://x.test/strategy/foo.md)",
		"`/strategy/foo.md`",
		"bare strategy/foo.md stay",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRewriteLinksLeavesFrontmatterBytes(t *testing.T) {
	raw := "---\ntype: Note\ntitle: 'It''s: quoted'\ndescription: See [Foo](/strategy/foo.md) for more.\n" +
		"resource: /strategy/foo.md\nsources: [{resource: docs/X.md}]\ntags: [a, \"2026\"]\n---\n\nBody [Foo](/strategy/foo.md).\n"
	out, n := RewriteLinks(raw, "notes/n.md", func(l Link) (string, bool) {
		if l.Resolved == "strategy/foo.md" {
			return "/marketing/foo.md", true
		}
		return "", false
	})
	want := strings.ReplaceAll(raw, "[Foo](/strategy/foo.md)", "[Foo](/marketing/foo.md)")
	if n != 2 || out != want {
		t.Errorf("n = %d, output:\n%s\nwant:\n%s", n, out, want)
	}
}

func TestFindMentions(t *testing.T) {
	text := "See strategy/foo.md and `/strategy/foo.md` and [Foo](/strategy/foo.md). " +
		"Not marketing/strategy/foo.md or strategy/foo.mdx or strategy/foo.md/x. End with strategy/foo.md."
	got := FindMentions(text, "notes/n.md", []string{"strategy/foo.md"})
	want := []Mention{
		{Path: "strategy/foo.md", InCode: false},
		{Path: "strategy/foo.md", InCode: true},
		{Path: "strategy/foo.md", InCode: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mentions = %+v, want %+v", got, want)
	}
}

func TestConceptLinks(t *testing.T) {
	c := &Concept{
		Path: "clients/a.md",
		Meta: ConceptMeta{Title: "A", Description: "Pairs with [B](/clients/b.md).", Resource: "/clients/c.md"},
		Body: "[B](b.md) and [D](/decisions/d.md) and [web](https://example.com).",
	}
	got := ConceptLinks(c)
	if want := []string{"clients/b.md", "decisions/d.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ConceptLinks = %v, want %v (resource is not a link)", got, want)
	}
}

func TestSuggest(t *testing.T) {
	paths := []string{"clients/acme.md", "instances/acme-deployment.md", "prospects/globex-jane-doe.md", "strategy/foo.md"}
	exists := pathSet(paths)
	aliases := map[string]string{"strategy/old.md": "marketing/new.md"}

	tests := []struct {
		raw, resolved string
		want          []string
	}{
		{"/partners/globex.md", "partners/globex.md", []string{"prospects/globex-jane-doe.md"}},
		{"/strategy/old.md", "strategy/old.md", []string{"marketing/new.md"}},
		{"strategy/foo.md", "clients/strategy/foo.md", []string{"strategy/foo.md"}},
		{"/partners/acme.md", "partners/acme.md", []string{"clients/acme.md", "instances/acme-deployment.md"}},
		{"/x/nothing-like-it.md", "x/nothing-like-it.md", nil},
	}
	for _, tt := range tests {
		if got := suggest(tt.raw, tt.resolved, paths, exists, aliases); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("suggest(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}
