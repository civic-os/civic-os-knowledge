package search

import (
	"strings"
	"sync"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
)

// Entry is an indexed concept for searching.
type Entry struct {
	Path     string
	Meta     bundle.ConceptMeta
	bodyLow  string // lowercased body text
	titleLow string // lowercased title
	descLow  string // lowercased description
}

// Result is a search result with a relevance score.
type Result struct {
	Path  string
	Meta  bundle.ConceptMeta
	Score int
}

// Index is an in-memory search index over OKF concepts.
type Index struct {
	mu      sync.RWMutex
	entries map[string]*Entry // keyed by path
}

// NewIndex creates an empty Index.
func NewIndex() *Index {
	return &Index{entries: make(map[string]*Entry)}
}

// BuildFromBundle populates the index from all concepts in a bundle.
func (idx *Index) BuildFromBundle(concepts []*bundle.Concept) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.entries = make(map[string]*Entry, len(concepts))
	for _, c := range concepts {
		idx.entries[c.Path] = &Entry{
			Path:     c.Path,
			Meta:     c.Meta,
			bodyLow:  strings.ToLower(c.Body),
			titleLow: strings.ToLower(c.Meta.Title),
			descLow:  strings.ToLower(c.Meta.Description),
		}
	}
}

// Add adds or updates a single concept in the index.
func (idx *Index) Add(c *bundle.Concept) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.entries[c.Path] = &Entry{
		Path:     c.Path,
		Meta:     c.Meta,
		bodyLow:  strings.ToLower(c.Body),
		titleLow: strings.ToLower(c.Meta.Title),
		descLow:  strings.ToLower(c.Meta.Description),
	}
}

// Remove removes a concept from the index.
func (idx *Index) Remove(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	delete(idx.entries, path)
}

// Search finds concepts matching the query with optional type and tag filters.
// Empty query returns all entries (filtered by type/tags if provided).
// Scoring: title match = 3, description match = 2, body match = 1.
func (idx *Index) Search(query, typeFilter string, tagFilters []string) []Result {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	queryLow := strings.ToLower(query)
	var results []Result

	for _, e := range idx.entries {
		// Apply type filter
		if typeFilter != "" && !strings.EqualFold(e.Meta.Type, typeFilter) {
			continue
		}

		// Apply tag filter (any match)
		if len(tagFilters) > 0 && !matchesAnyTag(e.Meta.Tags, tagFilters) {
			continue
		}

		// Score query match
		if query == "" {
			results = append(results, Result{Path: e.Path, Meta: e.Meta, Score: 1})
			continue
		}

		score := 0
		if strings.Contains(e.titleLow, queryLow) {
			score += 3
		}
		if strings.Contains(e.descLow, queryLow) {
			score += 2
		}
		if strings.Contains(e.bodyLow, queryLow) {
			score += 1
		}

		if score > 0 {
			results = append(results, Result{Path: e.Path, Meta: e.Meta, Score: score})
		}
	}

	// Sort by score descending, then path ascending for stability
	sortResults(results)
	return results
}

// List returns all indexed paths, optionally filtered by type.
func (idx *Index) List(typeFilter string) []Result {
	return idx.Search("", typeFilter, nil)
}

func matchesAnyTag(conceptTags, filterTags []string) bool {
	for _, ft := range filterTags {
		ftLow := strings.ToLower(ft)
		for _, ct := range conceptTags {
			if strings.EqualFold(ct, ftLow) {
				return true
			}
		}
	}
	return false
}

func sortResults(results []Result) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0; j-- {
			if results[j].Score > results[j-1].Score {
				results[j], results[j-1] = results[j-1], results[j]
			} else if results[j].Score == results[j-1].Score && results[j].Path < results[j-1].Path {
				results[j], results[j-1] = results[j-1], results[j]
			} else {
				break
			}
		}
	}
}
