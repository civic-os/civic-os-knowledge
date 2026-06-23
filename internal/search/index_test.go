package search

import (
	"testing"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
)

func testConcepts() []*bundle.Concept {
	return []*bundle.Concept{
		{
			Meta: bundle.ConceptMeta{
				Type:        "Client Profile",
				Title:       "Mott Park Recreation",
				Description: "Clubhouse reservation system with payment tracking.",
				Tags:        []string{"customer", "payments", "production"},
			},
			Body: "A recreation center management system serving the Mott Park community.",
			Path: "clients/mottpark.md",
		},
		{
			Meta: bundle.ConceptMeta{
				Type:        "Client Profile",
				Title:       "Genesee County Land Bank",
				Description: "Property management and demolition tracking.",
				Tags:        []string{"customer", "gis", "pilot"},
			},
			Body: "Manages vacant properties and coordinates demolition projects.",
			Path: "clients/gclb.md",
		},
		{
			Meta: bundle.ConceptMeta{
				Type:        "Decision Record",
				Title:       "Use Go for MCP Server",
				Description: "Go provides small binaries and built-in concurrency.",
				Tags:        []string{"architecture"},
			},
			Body: "We needed to choose a language for the knowledgebase server.",
			Path: "decisions/use-go.md",
		},
		{
			Meta: bundle.ConceptMeta{
				Type:        "Runbook",
				Title:       "Deploy to Production",
				Description: "Step-by-step production deployment procedure.",
				Tags:        []string{"operations", "production"},
			},
			Body: "Prerequisites: kubectl access to the cluster.",
			Path: "runbooks/deploy-prod.md",
		},
	}
}

func TestBuildAndSearch(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.Search("recreation", "", nil)
	if len(results) == 0 {
		t.Fatal("expected results for 'recreation'")
	}
	if results[0].Path != "clients/mottpark.md" {
		t.Errorf("first result = %q, want clients/mottpark.md", results[0].Path)
	}
}

func TestSearchTitleWeight(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	// "Mott Park" appears in title — should score higher than body matches
	results := idx.Search("mott park", "", nil)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Score < 3 {
		t.Errorf("title match score = %d, expected >= 3", results[0].Score)
	}
}

func TestSearchTypeFilter(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.Search("", "Client Profile", nil)
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Meta.Type != "Client Profile" {
			t.Errorf("result type = %q, want Client Profile", r.Meta.Type)
		}
	}
}

func TestSearchTagFilter(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.Search("", "", []string{"production"})
	if len(results) != 2 {
		t.Errorf("got %d results, want 2 (mottpark + deploy-prod)", len(results))
	}
}

func TestSearchCombinedFilters(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.Search("", "Client Profile", []string{"production"})
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].Path != "clients/mottpark.md" {
		t.Errorf("result = %q, want clients/mottpark.md", results[0].Path)
	}
}

func TestSearchEmpty(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.Search("xyznonexistent", "", nil)
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.Search("MOTT PARK", "", nil)
	if len(results) == 0 {
		t.Fatal("case-insensitive search should find results")
	}
}

func TestListAll(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.List("")
	if len(results) != 4 {
		t.Errorf("got %d results, want 4", len(results))
	}
}

func TestListByType(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.List("Runbook")
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
}

func TestAddAndRemove(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	// Add new concept
	idx.Add(&bundle.Concept{
		Meta: bundle.ConceptMeta{Type: "Note", Title: "New Note"},
		Body: "Fresh content.",
		Path: "notes/new.md",
	})

	results := idx.List("")
	if len(results) != 5 {
		t.Errorf("after add: got %d, want 5", len(results))
	}

	// Remove it
	idx.Remove("notes/new.md")
	results = idx.List("")
	if len(results) != 4 {
		t.Errorf("after remove: got %d, want 4", len(results))
	}
}

func TestEmptyIndex(t *testing.T) {
	idx := NewIndex()

	results := idx.Search("anything", "", nil)
	if len(results) != 0 {
		t.Errorf("empty index should return 0 results")
	}
}

func TestTagFilterCaseInsensitive(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	results := idx.Search("", "", []string{"PAYMENTS"})
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
}

func TestSearchMultiWordAcrossFields(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	// "mott" in title, "reservation" in description — should match mottpark
	results := idx.Search("mott reservation", "", nil)
	if len(results) == 0 {
		t.Fatal("expected results for multi-word cross-field query")
	}
	if results[0].Path != "clients/mottpark.md" {
		t.Errorf("first result = %q, want clients/mottpark.md", results[0].Path)
	}
}

func TestSearchMultiWordPartialMatch(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	// "recreation" matches mottpark, "demolition" matches gclb — both should appear
	results := idx.Search("recreation demolition", "", nil)
	if len(results) < 2 {
		t.Fatalf("got %d results, want at least 2", len(results))
	}
	paths := map[string]bool{}
	for _, r := range results {
		paths[r.Path] = true
	}
	if !paths["clients/mottpark.md"] {
		t.Error("expected mottpark in results")
	}
	if !paths["clients/gclb.md"] {
		t.Error("expected gclb in results")
	}
}

func TestSearchMultiWordRanking(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	// "property" and "demolition" both appear in gclb — should rank first
	results := idx.Search("property demolition", "", nil)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Path != "clients/gclb.md" {
		t.Errorf("first result = %q, want clients/gclb.md", results[0].Path)
	}
	// Concepts matching only one word should rank lower
	if len(results) > 1 && results[1].Score >= results[0].Score {
		t.Errorf("two-word match (%d) should score higher than one-word match (%d)",
			results[0].Score, results[1].Score)
	}
}

func TestSearchWordOrderIrrelevant(t *testing.T) {
	idx := NewIndex()
	idx.BuildFromBundle(testConcepts())

	r1 := idx.Search("mott park", "", nil)
	r2 := idx.Search("park mott", "", nil)

	if len(r1) != len(r2) {
		t.Fatalf("different result counts: %d vs %d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i].Path != r2[i].Path || r1[i].Score != r2[i].Score {
			t.Errorf("result[%d] differs: %v vs %v", i, r1[i], r2[i])
		}
	}
}
