package bundle

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// updateBody bumps a concept's version with a new body.
func updateBody(t *testing.T, b *Bundle, p, body string) {
	t.Helper()
	c, err := b.Read(p)
	if err != nil {
		t.Fatal(err)
	}
	c.Body = body
	if _, err := b.Update(c, 0); err != nil {
		t.Fatalf("update %s: %v", p, err)
	}
}

func readRaw(t *testing.T, b *Bundle, p string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(b.rootDir, p))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMoveAcrossDirectories(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "strategy/foo.md", "v1")
	updateBody(t, b, "strategy/foo.md", "v2")
	updateBody(t, b, "strategy/foo.md", "v3")

	res, err := b.MoveBatch([]MoveItem{{Path: "strategy/foo.md", NewPath: "marketing/foo.md", Version: 3}})
	if err != nil {
		t.Fatal(err)
	}

	m := res.Moved[0]
	if m.From != "strategy/foo.md" || m.Concept.Path != "marketing/foo.md" || m.Concept.Version != 4 || m.Carried != 2 {
		t.Errorf("moved = %+v (concept %s v%d)", m, m.Concept.Path, m.Concept.Version)
	}

	read, err := b.Read("marketing/foo.md")
	if err != nil {
		t.Fatal(err)
	}
	if read.Body != "v3" || read.Version != 4 || !reflect.DeepEqual(read.Meta.Aliases, []string{"strategy/foo.md"}) {
		t.Errorf("read body %q v%d aliases %v", read.Body, read.Version, read.Meta.Aliases)
	}
	if read.Meta.Timestamp != "" {
		t.Errorf("move must not touch timestamp, got %q", read.Meta.Timestamp)
	}

	// History moved and renamed; the pre-move content is snapshot 3.
	hist, _ := b.History("marketing/foo.md")
	if !reflect.DeepEqual(hist, []int{1, 2, 3}) {
		t.Errorf("history = %v, want [1 2 3]", hist)
	}
	if _, err := os.Stat(filepath.Join(b.verDir, "marketing/foo/foo.1.md")); err != nil {
		t.Errorf("carried snapshot missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.verDir, "strategy")); !os.IsNotExist(err) {
		t.Error("old versions directory should be removed once empty")
	}
	if _, err := os.Stat(filepath.Join(b.rootDir, "strategy")); !os.IsNotExist(err) {
		t.Error("old bundle directory should be removed once empty")
	}

	diff, err := b.Diff("marketing/foo.md", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+aliases: [strategy/foo.md]") {
		t.Errorf("diff should show the alias being added:\n%s", diff)
	}

	wantRemoved := []string{"strategy/foo.md"}
	if !reflect.DeepEqual(res.Removed, wantRemoved) || !reflect.DeepEqual(res.Written, []string{"marketing/foo.md"}) {
		t.Errorf("written %v removed %v", res.Written, res.Removed)
	}
	if !reflect.DeepEqual(res.VersionsRemoved, []string{"strategy/foo/foo.1.md", "strategy/foo/foo.2.md"}) {
		t.Errorf("versions removed = %v", res.VersionsRemoved)
	}
	if !reflect.DeepEqual(res.VersionsWritten, []string{"marketing/foo/foo.1.md", "marketing/foo/foo.2.md", "marketing/foo/foo.3.md"}) {
		t.Errorf("versions written = %v", res.VersionsWritten)
	}
}

func TestMoveRenameWithinDirectory(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "research/old-name.md", "v1")
	updateBody(t, b, "research/old-name.md", "v2")

	if _, err := b.MoveBatch([]MoveItem{{Path: "research/old-name.md", NewPath: "research/new-name.md"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(b.verDir, "research/new-name/new-name.1.md")); err != nil {
		t.Errorf("renamed snapshot missing: %v", err)
	}
	if read, _ := b.Read("research/new-name.md"); read.Version != 3 {
		t.Errorf("version = %d, want 3", read.Version)
	}
}

func TestMoveRewritesInboundLinks(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "strategy/foo.md", "Self [link](/strategy/foo.md).")
	mustCreate(t, b, "strategy/bar.md", "Bar.")
	raw := "---\ntype: Note\ntitle: 'A: quoted'\ndescription: Uses [Foo](/strategy/foo.md).\ntags: [x, \"2026\"]\n---\n\n" +
		"| Col |\n|-----|\n| [Foo](/strategy/foo.md) |\n\n> [Foo again](/strategy/foo.md#x) and [Bar](/strategy/bar.md)\n\n" +
		"Code `strategy/foo.md` stays.\n"
	if err := os.MkdirAll(filepath.Join(b.rootDir, "clients"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.rootDir, "clients/a.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	mustCreate(t, b, "clients/untouched.md", "No links here.")

	res, err := b.MoveBatch([]MoveItem{{Path: "strategy/foo.md", NewPath: "marketing/foo.md"}})
	if err != nil {
		t.Fatal(err)
	}

	want := strings.NewReplacer(
		"[Foo](/strategy/foo.md)", "[Foo](/marketing/foo.md)",
		"[Foo again](/strategy/foo.md#x)", "[Foo again](/marketing/foo.md#x)",
	).Replace(raw)
	if got := readRaw(t, b, "clients/a.md"); got != want {
		t.Errorf("relinked file:\n%s\nwant:\n%s", got, want)
	}
	if read, _ := b.Read("clients/a.md"); read.Version != 2 {
		t.Errorf("relinked version = %d, want 2", read.Version)
	}
	if read, _ := b.Read("marketing/foo.md"); read.Body != "Self [link](/marketing/foo.md)." {
		t.Errorf("self-link not rewritten: %q", read.Body)
	}
	if read, _ := b.Read("clients/untouched.md"); read.Version != 1 {
		t.Error("concepts without links must not be rewritten")
	}

	if len(res.Relinked) != 1 || res.Relinked[0].Path != "clients/a.md" {
		t.Errorf("relinked = %v", res.Relinked)
	}
	if got := res.Mentions["strategy/foo.md"]; !reflect.DeepEqual(got, []string{"clients/a.md"}) {
		t.Errorf("mentions = %v", res.Mentions)
	}
}

func TestMoveBatchRewritesEachConceptOnce(t *testing.T) {
	b := testBundle(t)
	for _, p := range []string{"strategy/a.md", "strategy/b.md", "strategy/c.md"} {
		mustCreate(t, b, p, "x")
	}
	mustCreate(t, b, "runbooks/guide.md", "[A](/strategy/a.md) [B](/strategy/b.md) [C](/strategy/c.md) [A](/strategy/a.md)")
	updateBody(t, b, "strategy/a.md", "Links [B](/strategy/b.md).")

	res, err := b.MoveBatch([]MoveItem{
		{Path: "strategy/a.md", NewPath: "marketing/a.md"},
		{Path: "strategy/b.md", NewPath: "marketing/b.md"},
		{Path: "strategy/c.md", NewPath: "marketing/c.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	guide, _ := b.Read("runbooks/guide.md")
	if guide.Version != 2 || guide.Body != "[A](/marketing/a.md) [B](/marketing/b.md) [C](/marketing/c.md) [A](/marketing/a.md)" {
		t.Errorf("guide v%d body %q", guide.Version, guide.Body)
	}
	if a, _ := b.Read("marketing/a.md"); a.Body != "Links [B](/marketing/b.md)." || a.Version != 3 {
		t.Errorf("links between moved concepts: v%d %q", a.Version, a.Body)
	}
	if len(res.Relinked) != 1 {
		t.Errorf("relinked %d concepts, want 1", len(res.Relinked))
	}
}

func TestMoveBatchValidation(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "a/one.md", "1")
	mustCreate(t, b, "a/two.md", "2")
	updateBody(t, b, "a/one.md", "1b")

	tests := []struct {
		name  string
		items []MoveItem
		want  string
	}{
		{"empty", nil, "no moves"},
		{"same path", []MoveItem{{Path: "a/one.md", NewPath: "a/one.md"}}, "same as the current path"},
		{"invalid target", []MoveItem{{Path: "a/one.md", NewPath: "../x.md"}}, "invalid path"},
		{"missing source", []MoveItem{{Path: "a/nope.md", NewPath: "b/nope.md"}}, "not found"},
		{"existing target", []MoveItem{{Path: "a/one.md", NewPath: "a/two.md"}}, "already exists"},
		{"duplicate source", []MoveItem{{Path: "a/one.md", NewPath: "b/x.md"}, {Path: "a/one.md", NewPath: "b/y.md"}}, "more than once"},
		{"duplicate target", []MoveItem{{Path: "a/one.md", NewPath: "b/x.md"}, {Path: "a/two.md", NewPath: "b/x.md"}}, "more than one concept"},
		{"chain", []MoveItem{{Path: "a/one.md", NewPath: "b/one.md"}, {Path: "b/one.md", NewPath: "c/one.md"}}, "chains and swaps"},
		{"swap", []MoveItem{{Path: "a/one.md", NewPath: "a/two.md"}, {Path: "a/two.md", NewPath: "a/one.md"}}, "chains and swaps"},
		{"conflict", []MoveItem{{Path: "a/one.md", NewPath: "b/one.md", Version: 1}}, "version conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := b.MoveBatch(tt.items)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
	if _, err := b.MoveBatch([]MoveItem{{Path: "a/one.md", NewPath: "b/one.md", Version: 1}}); !errors.Is(err, ErrConflict) {
		t.Errorf("conflict should wrap ErrConflict: %v", err)
	}

	// Orphaned history at the target is never merged.
	if err := os.MkdirAll(filepath.Join(b.verDir, "b/orphan"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := b.MoveBatch([]MoveItem{{Path: "a/one.md", NewPath: "b/orphan.md"}}); err == nil || !strings.Contains(err.Error(), "version history already exists") {
		t.Errorf("orphaned history: %v", err)
	}

	// Nothing changed.
	for _, p := range []string{"a/one.md", "a/two.md"} {
		if _, err := os.Stat(filepath.Join(b.rootDir, p)); err != nil {
			t.Errorf("%s should be untouched: %v", p, err)
		}
	}
	if read, _ := b.Read("a/one.md"); read.Version != 2 || len(read.Meta.Aliases) != 0 {
		t.Errorf("a/one.md changed: v%d aliases %v", read.Version, read.Meta.Aliases)
	}
}

func TestAliasResolution(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "strategy/foo.md", "Foo.")
	if _, err := b.MoveBatch([]MoveItem{{Path: "strategy/foo.md", NewPath: "marketing/foo.md"}}); err != nil {
		t.Fatal(err)
	}

	// Reads follow the alias.
	c, err := b.Read("strategy/foo.md")
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != "marketing/foo.md" || c.ResolvedFrom != "strategy/foo.md" {
		t.Errorf("read via alias: path %q resolved from %q", c.Path, c.ResolvedFrom)
	}
	if hist, _ := b.History("strategy/foo.md"); !reflect.DeepEqual(hist, []int{1}) {
		t.Errorf("history via alias = %v", hist)
	}

	// Writes don't.
	var moved *MovedError
	if _, err := b.Update(&Concept{Meta: ConceptMeta{Type: "Note"}, Path: "strategy/foo.md"}, 0); !errors.As(err, &moved) || moved.Current != "marketing/foo.md" {
		t.Errorf("update via alias: %v", err)
	}
	if _, err := b.MoveBatch([]MoveItem{{Path: "strategy/foo.md", NewPath: "x/foo.md"}}); !errors.As(err, &moved) {
		t.Errorf("move via alias: %v", err)
	}

	// A new link to the old path is normalized to the current one.
	c2 := &Concept{Meta: ConceptMeta{Type: "Note"}, Body: "[Foo](/strategy/foo.md#top)", Path: "clients/a.md"}
	if _, err := b.Create(c2); err != nil {
		t.Fatal(err)
	}
	if c2.Body != "[Foo](/marketing/foo.md#top)" {
		t.Errorf("link to alias not normalized: %q", c2.Body)
	}
}

func TestAliasPrecedenceAndCleanup(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "strategy/foo.md", "Original.")
	if _, err := b.MoveBatch([]MoveItem{{Path: "strategy/foo.md", NewPath: "marketing/foo.md"}}); err != nil {
		t.Fatal(err)
	}

	// A new concept at the old path wins over the alias.
	mustCreate(t, b, "strategy/foo.md", "Newcomer.")
	if c, _ := b.Read("strategy/foo.md"); c.Body != "Newcomer." || c.ResolvedFrom != "" {
		t.Errorf("file should win over alias: %q (resolved from %q)", c.Body, c.ResolvedFrom)
	}

	// When the newcomer moves away, the old concept's stale alias is dropped
	// and the newcomer takes the alias over.
	res, err := b.MoveBatch([]MoveItem{{Path: "strategy/foo.md", NewPath: "archive/foo.md"}})
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := b.Read("marketing/foo.md")
	if len(orig.Meta.Aliases) != 0 || orig.Version != 3 {
		t.Errorf("stale alias should be removed with a version bump: v%d aliases %v", orig.Version, orig.Meta.Aliases)
	}
	if len(res.Relinked) != 1 || res.Relinked[0].Path != "marketing/foo.md" {
		t.Errorf("relinked = %v", res.Relinked)
	}
	if c, _ := b.Read("strategy/foo.md"); c.Path != "archive/foo.md" {
		t.Errorf("alias should now resolve to archive/foo.md, got %s", c.Path)
	}
}

func TestMoveBackToFormerPath(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "strategy/foo.md", "Foo.")
	if _, err := b.MoveBatch([]MoveItem{{Path: "strategy/foo.md", NewPath: "marketing/foo.md"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.MoveBatch([]MoveItem{{Path: "marketing/foo.md", NewPath: "strategy/foo.md"}}); err != nil {
		t.Fatal(err)
	}
	c, _ := b.Read("strategy/foo.md")
	if !reflect.DeepEqual(c.Meta.Aliases, []string{"marketing/foo.md"}) || c.Version != 3 {
		t.Errorf("v%d aliases %v, want [marketing/foo.md]", c.Version, c.Meta.Aliases)
	}
	if hist, _ := b.History("strategy/foo.md"); !reflect.DeepEqual(hist, []int{1, 2}) {
		t.Errorf("history = %v, want [1 2]", hist)
	}
}

func TestMoveCarriesDottedSlugHistory(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "research/k8s-v1.2.md", "v1")
	updateBody(t, b, "research/k8s-v1.2.md", "v2")
	if _, err := b.MoveBatch([]MoveItem{{Path: "research/k8s-v1.2.md", NewPath: "infrastructure/k8s-v1.3.md"}}); err != nil {
		t.Fatal(err)
	}
	if hist, _ := b.History("infrastructure/k8s-v1.3.md"); !reflect.DeepEqual(hist, []int{1, 2}) {
		t.Errorf("history = %v, want [1 2]", hist)
	}
}

func TestLinksOfAndAudit(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "strategy/foo.md", "Foo.")
	mustCreate(t, b, "clients/a.md", "[Foo](/strategy/foo.md) [Foo](/strategy/foo.md) [web](https://x.test)")
	mustCreate(t, b, "clients/b.md", "Mentions `strategy/foo.md` in code.")
	raw := "---\ntype: Note\n---\n\n[Gone](/partners/globex.md) [Foo](strategy/foo.md)\n\nPlain clients/a.md here.\n"
	if err := os.WriteFile(filepath.Join(b.rootDir, "clients/c.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := b.LinksOf("strategy/foo.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Inbound) != 1 || info.Inbound[0].Path != "clients/a.md" || info.Inbound[0].Count != 2 {
		t.Errorf("inbound = %+v", info.Inbound)
	}
	if len(info.Mentions) != 1 || info.Mentions[0].Path != "clients/b.md" || !info.Mentions[0].InCode {
		t.Errorf("mentions = %+v", info.Mentions)
	}

	a, _ := b.LinksOf("clients/a.md")
	if len(a.Outbound) != 1 || a.Outbound[0].Count != 2 || a.External != 1 {
		t.Errorf("outbound = %+v external %d", a.Outbound, a.External)
	}

	audit, err := b.AuditLinks()
	if err != nil {
		t.Fatal(err)
	}
	// clients/c.md's relative "strategy/foo.md" resolves to clients/strategy/foo.md: broken.
	if len(audit.Broken) != 2 {
		t.Errorf("broken = %+v", audit.Broken)
	}
	if len(audit.Mentions) != 1 || audit.Mentions[0].Source != "clients/c.md" || audit.Mentions[0].Target != "clients/a.md" {
		t.Errorf("mentions = %+v", audit.Mentions)
	}
	if audit.Concepts != 4 || audit.Links != 4 || audit.External != 1 {
		t.Errorf("counts: %d concepts %d links %d external", audit.Concepts, audit.Links, audit.External)
	}
}
