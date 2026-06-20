package bundle

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

func testBundle(t *testing.T) *Bundle {
	t.Helper()
	dir := t.TempDir()
	b, err := NewBundle(dir + "/bundle")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCreateAndRead(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{
			Type:        "Client Profile",
			Title:       "Test Client",
			Description: "A test client.",
			Tags:        []string{"test"},
			Timestamp:   "2026-06-19",
		},
		Body: "# Overview\n\nTest content.",
		Path: "clients/test.md",
	}

	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	read, err := b.Read("clients/test.md")
	if err != nil {
		t.Fatal(err)
	}

	if read.Meta.Type != "Client Profile" {
		t.Errorf("type = %q", read.Meta.Type)
	}
	if read.Meta.Title != "Test Client" {
		t.Errorf("title = %q", read.Meta.Title)
	}
	if !strings.Contains(read.Body, "Test content.") {
		t.Errorf("body = %q", read.Body)
	}
	if read.Version != 1 {
		t.Errorf("version = %d, want 1", read.Version)
	}
}

func TestCreateDuplicate(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Title: "First"},
		Path: "notes/a.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	err := b.Create(c)
	if err == nil {
		t.Fatal("expected error for duplicate create")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestList(t *testing.T) {
	b := testBundle(t)

	for _, path := range []string{"clients/a.md", "clients/b.md", "runbooks/deploy.md"} {
		c := &Concept{
			Meta: ConceptMeta{Type: "Note", Title: path},
			Path: path,
		}
		if err := b.Create(c); err != nil {
			t.Fatal(err)
		}
	}

	concepts, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(concepts) != 3 {
		t.Errorf("got %d concepts, want 3", len(concepts))
	}
	for _, c := range concepts {
		if c.Version != 1 {
			t.Errorf("concept %s version = %d, want 1", c.Path, c.Version)
		}
	}
}

func TestUpdateAndHistory(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Title: "V1"},
		Body: "Version 1 content.",
		Path: "notes/a.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	c.Meta.Title = "V2"
	c.Body = "Version 2 content."
	if err := b.Update(c, 1); err != nil {
		t.Fatal(err)
	}

	// Read should return v2
	read, err := b.Read("notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if read.Meta.Title != "V2" {
		t.Errorf("title = %q, want V2", read.Meta.Title)
	}
	if read.Version != 2 {
		t.Errorf("version = %d, want 2", read.Version)
	}

	// History should have one snapshot (version 1)
	hist, err := b.History("notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Errorf("history length = %d, want 1", len(hist))
	}
	if len(hist) > 0 && hist[0] != 1 {
		t.Errorf("history[0] = %d, want 1", hist[0])
	}
}

func TestUpdateNonExistent(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Note"},
		Path: "notes/missing.md",
	}
	err := b.Update(c, 0)
	if err == nil {
		t.Fatal("expected error for update of non-existent file")
	}
}

func TestUpdateConflict(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Title: "V1"},
		Body: "Initial.",
		Path: "notes/a.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	// First update at version 1 should succeed
	c.Meta.Title = "V2"
	c.Body = "Updated by session A."
	if err := b.Update(c, 1); err != nil {
		t.Fatal(err)
	}

	// Second update with stale version 1 should fail
	c.Meta.Title = "V2-stale"
	c.Body = "Updated by session B (stale)."
	err := b.Update(c, 1)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}

	// Update with correct version 2 should succeed
	c.Meta.Title = "V3"
	c.Body = "Updated by session B (correct)."
	if err := b.Update(c, 2); err != nil {
		t.Fatal(err)
	}

	read, err := b.Read("notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if read.Version != 3 {
		t.Errorf("version = %d, want 3", read.Version)
	}
	if read.Meta.Title != "V3" {
		t.Errorf("title = %q, want V3", read.Meta.Title)
	}
}

func TestUpdateNoVersionCheck(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Title: "V1"},
		Path: "notes/a.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	// Update with version 0 skips check (backwards compat)
	c.Meta.Title = "V2"
	if err := b.Update(c, 0); err != nil {
		t.Fatal(err)
	}

	read, err := b.Read("notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if read.Version != 2 {
		t.Errorf("version = %d, want 2", read.Version)
	}
}

func TestVersionBasedSnapshotNaming(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Title: "V1"},
		Body: "Content v1.",
		Path: "notes/a.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	// Two updates
	c.Meta.Title = "V2"
	if err := b.Update(c, 0); err != nil {
		t.Fatal(err)
	}
	c.Meta.Title = "V3"
	if err := b.Update(c, 0); err != nil {
		t.Fatal(err)
	}

	// Check snapshot files exist with version-based naming
	verDir := b.verDir + "/notes/a"
	entries, err := os.ReadDir(verDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 snapshot files, got %d", len(entries))
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["a.1.md"] {
		t.Error("missing snapshot a.1.md")
	}
	if !names["a.2.md"] {
		t.Error("missing snapshot a.2.md")
	}
}

func TestDiff(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Title: "Original"},
		Body: "Original body.",
		Path: "notes/a.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	c.Meta.Title = "Updated"
	c.Body = "Updated body."
	if err := b.Update(c, 0); err != nil {
		t.Fatal(err)
	}

	hist, err := b.History("notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) == 0 {
		t.Fatal("no history")
	}

	diff, err := b.Diff("notes/a.md", hist[0])
	if err != nil {
		t.Fatal(err)
	}
	if diff == "" {
		t.Error("diff is empty")
	}
	if !strings.Contains(diff, "Original") || !strings.Contains(diff, "Updated") {
		t.Errorf("diff doesn't contain expected content: %s", diff)
	}
}

func TestHistoryEmpty(t *testing.T) {
	b := testBundle(t)

	hist, err := b.History("notes/nonexistent.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 0 {
		t.Errorf("history should be empty, got %v", hist)
	}
}

func TestReadNonExistent(t *testing.T) {
	b := testBundle(t)

	_, err := b.Read("missing.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestConcurrentAccess(t *testing.T) {
	b := testBundle(t)

	// Create initial concept
	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Title: "Concurrent"},
		Body: "Initial.",
		Path: "notes/concurrent.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	// 10 concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := b.Read("notes/concurrent.md")
			if err != nil {
				errs <- err
			}
		}()
	}

	// 10 concurrent listers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := b.List()
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestNestedDirectories(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Runbook", Title: "Deep"},
		Path: "ops/deeply/nested/runbook.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	read, err := b.Read("ops/deeply/nested/runbook.md")
	if err != nil {
		t.Fatal(err)
	}
	if read.Meta.Title != "Deep" {
		t.Errorf("title = %q", read.Meta.Title)
	}
}

func TestListSkipsNonMarkdown(t *testing.T) {
	b := testBundle(t)

	// Create a markdown concept
	c := &Concept{
		Meta: ConceptMeta{Type: "Note"},
		Path: "notes/a.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	// Write a non-markdown file directly
	if err := os.WriteFile(b.rootDir+"/notes/readme.txt", []byte("not a concept"), 0o644); err != nil {
		t.Fatal(err)
	}

	concepts, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(concepts) != 1 {
		t.Errorf("got %d concepts, want 1 (should skip .txt)", len(concepts))
	}
}

func TestHistoryReturnsVersionNumbers(t *testing.T) {
	b := testBundle(t)

	c := &Concept{
		Meta: ConceptMeta{Type: "Note", Title: "V1"},
		Path: "notes/a.md",
	}
	if err := b.Create(c); err != nil {
		t.Fatal(err)
	}

	// Three updates
	for i := 2; i <= 4; i++ {
		c.Meta.Title = "V" + string(rune('0'+i))
		if err := b.Update(c, 0); err != nil {
			t.Fatal(err)
		}
	}

	hist, err := b.History("notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(hist))
	}
	// Should be sorted: 1, 2, 3
	for i, expected := range []int{1, 2, 3} {
		if hist[i] != expected {
			t.Errorf("hist[%d] = %d, want %d", i, hist[i], expected)
		}
	}
}
