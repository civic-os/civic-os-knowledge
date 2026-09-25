package bundle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustCreate creates a Note concept at p with the given body.
func mustCreate(t *testing.T, b *Bundle, p, body string) *Concept {
	t.Helper()
	c := &Concept{Meta: ConceptMeta{Type: "Note", Title: strings.TrimSuffix(filepath.Base(p), ".md")}, Body: body, Path: p}
	if _, err := b.Create(c); err != nil {
		t.Fatalf("create %s: %v", p, err)
	}
	return c
}

func TestCreateNormalizesLinks(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "clients/b.md", "B.")
	mustCreate(t, b, "decisions/d.md", "D.")

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Description: "See [D](../decisions/d.md)."},
		Body: "[B](b.md) [B again](/clients/../clients/b.md#x) [web](https://example.com) [self](a.md)",
		Path: "clients/a.md",
	}
	report, err := b.Create(c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Meta.Description != "See [D](/decisions/d.md)." {
		t.Errorf("description = %q", c.Meta.Description)
	}
	want := "[B](/clients/b.md) [B again](/clients/b.md#x) [web](https://example.com) [self](/clients/a.md)"
	if c.Body != want {
		t.Errorf("body = %q, want %q", c.Body, want)
	}
	if len(report.Normalized) != 4 {
		t.Errorf("normalized = %+v, want 4 changes", report.Normalized)
	}
	read, _ := b.Read("clients/a.md")
	if read.Body != want {
		t.Errorf("stored body = %q", read.Body)
	}
}

func TestCreateRejectsBrokenLinks(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "prospects/globex-jane-doe.md", "Globex.")

	c := &Concept{
		Meta: ConceptMeta{Type: "Note"},
		Body: "[Globex](/partners/globex.md) and [ok](/prospects/globex-jane-doe.md) and [ext](https://nowhere.example/x.md)",
		Path: "company/team.md",
	}
	_, err := b.Create(c)
	var broken *BrokenLinksError
	if !errors.As(err, &broken) {
		t.Fatalf("expected BrokenLinksError, got %v", err)
	}
	if len(broken.Links) != 1 || broken.Links[0].Target != "/partners/globex.md" {
		t.Fatalf("broken = %+v", broken.Links)
	}
	if s := broken.Links[0].Suggestions; len(s) == 0 || s[0] != "prospects/globex-jane-doe.md" {
		t.Errorf("suggestions = %v", s)
	}
	if _, err := os.Stat(filepath.Join(b.rootDir, "company/team.md")); !os.IsNotExist(err) {
		t.Error("rejected create must not write the file")
	}
}

func TestUpdateLinkRatchet(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "clients/b.md", "B.")

	// Seed a concept that already has a broken link, as in the live KB.
	raw := "---\ntype: Note\ntitle: A\n---\n\nSee [gone](/partners/gone.md).\n"
	if err := os.MkdirAll(filepath.Join(b.rootDir, "clients"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.rootDir, "clients/a.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	// Keeping the pre-existing broken link warns but succeeds.
	c, _ := b.Read("clients/a.md")
	c.Meta.Title = "A2"
	report, err := b.Update(c, 0)
	if err != nil {
		t.Fatalf("update keeping pre-existing broken link: %v", err)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "/partners/gone.md") {
		t.Errorf("warnings = %v", report.Warnings)
	}

	// Adding a new broken link is rejected and nothing is written.
	c, _ = b.Read("clients/a.md")
	c.Body += "\nAnd [new](/partners/new.md)."
	_, err = b.Update(c, 0)
	var broken *BrokenLinksError
	if !errors.As(err, &broken) || len(broken.Links) != 1 || broken.Links[0].Target != "/partners/new.md" {
		t.Fatalf("expected one new broken link, got %v", err)
	}
	if read, _ := b.Read("clients/a.md"); read.Version != 2 {
		t.Errorf("version = %d, rejected update must not snapshot or write", read.Version)
	}
}

func TestLinkWarningsForMentions(t *testing.T) {
	b := testBundle(t)
	mustCreate(t, b, "clients/b.md", "B.")

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Resource: "/clients/b.md"},
		Body: "Plain clients/b.md and code `clients/b.md` and [linked](/clients/b.md).",
		Path: "clients/a.md",
	}
	report, err := b.Create(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "plain-text mention of clients/b.md") {
		t.Errorf("warnings = %v, want one plain-text mention warning", report.Warnings)
	}
}

func TestValidatePath(t *testing.T) {
	valid := []string{"a.md", "clients/acme.md", "research/k8s-v1.2.md"}
	invalid := []string{"", "/clients/a.md", "../x.md", "clients/../x.md", "./a.md", "clients//a.md",
		"clients/.hidden.md", ".versions/a.md", "a.txt", `clients\a.md`, ".md"}
	for _, p := range valid {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) = %v, want nil", p, err)
		}
	}
	for _, p := range invalid {
		if ValidatePath(p) == nil {
			t.Errorf("ValidatePath(%q) = nil, want error", p)
		}
	}

	b := testBundle(t)
	if _, err := b.Create(&Concept{Meta: ConceptMeta{Type: "Note"}, Path: "../escape.md"}); err == nil {
		t.Error("create outside the bundle should fail")
	}
}

func TestDottedSlugVersioning(t *testing.T) {
	b := testBundle(t)
	c := mustCreate(t, b, "research/k8s-v1.2.md", "v1")
	for i := 0; i < 2; i++ {
		c.Body += "."
		if _, err := b.Update(c, 0); err != nil {
			t.Fatal(err)
		}
	}
	read, _ := b.Read("research/k8s-v1.2.md")
	if read.Version != 3 {
		t.Errorf("version = %d, want 3", read.Version)
	}
	hist, _ := b.History("research/k8s-v1.2.md")
	if len(hist) != 2 || hist[0] != 1 || hist[1] != 2 {
		t.Errorf("history = %v, want [1 2]", hist)
	}
}

// TestDiffIsLineAccurate guards against the go-diff line-mode bug that
// scattered frontmatter lines through the body of multi-line diffs.
func TestDiffIsLineAccurate(t *testing.T) {
	b := testBundle(t)
	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, "Line "+strings.Repeat("x", i%7)+" number "+string(rune('A'+i%26)))
	}
	c := mustCreate(t, b, "notes/a.md", strings.Join(lines, "\n"))

	lines[4] = "Changed line five"
	lines[22] = "Changed line twenty-three"
	c.Body = strings.Join(lines, "\n") + "\nAppended."
	if _, err := b.Update(c, 0); err != nil {
		t.Fatal(err)
	}

	diff, err := b.Diff("notes/a.md", 1)
	if err != nil {
		t.Fatal(err)
	}
	var added, removed []string
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		case strings.HasPrefix(l, "+"):
			added = append(added, l[1:])
		case strings.HasPrefix(l, "-"):
			removed = append(removed, l[1:])
		}
	}
	wantAdded := []string{"Changed line five", "Changed line twenty-three", "Appended."}
	if strings.Join(added, "|") != strings.Join(wantAdded, "|") {
		t.Errorf("added = %q, want %q\n%s", added, wantAdded, diff)
	}
	if len(removed) != 2 {
		t.Errorf("removed = %q, want 2 lines\n%s", removed, diff)
	}
	if strings.Contains(diff, "\x1b[") {
		t.Error("diff contains ANSI escape codes")
	}
	if !strings.Contains(diff, "notes/a.md@v1") {
		t.Errorf("diff header missing version label:\n%s", diff)
	}
}
