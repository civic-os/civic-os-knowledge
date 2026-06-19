package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Bundle manages a directory of OKF concept files with versioning.
type Bundle struct {
	mu      sync.RWMutex
	rootDir string // path to bundle directory
	verDir  string // path to .versions directory
}

// NewBundle creates a Bundle rooted at the given directory.
// It creates the bundle and versions directories if they don't exist.
func NewBundle(rootDir string) (*Bundle, error) {
	verDir := filepath.Join(filepath.Dir(rootDir), ".versions")
	for _, dir := range []string{rootDir, verDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return &Bundle{rootDir: rootDir, verDir: verDir}, nil
}

// Read reads and parses a concept file by its relative path.
func (b *Bundle) Read(path string) (*Concept, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	data, err := os.ReadFile(filepath.Join(b.rootDir, path))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseConcept(data, path)
}

// List returns all concept files in the bundle directory (recursively).
func (b *Bundle) List() ([]*Concept, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.listUnlocked()
}

func (b *Bundle) listUnlocked() ([]*Concept, error) {
	var concepts []*Concept
	err := filepath.Walk(b.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, err := filepath.Rel(b.rootDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		c, err := ParseConcept(data, rel)
		if err != nil {
			// Skip files that fail to parse (e.g., index.md without frontmatter)
			return nil
		}
		concepts = append(concepts, c)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list bundle: %w", err)
	}
	return concepts, nil
}

// Create writes a new concept file. Returns an error if the file already exists.
func (b *Bundle) Create(c *Concept) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	fullPath := filepath.Join(b.rootDir, c.Path)
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("concept already exists: %s", c.Path)
	}

	return b.writeFile(c)
}

// Update overwrites an existing concept file, creating a version snapshot first.
func (b *Bundle) Update(c *Concept) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	fullPath := filepath.Join(b.rootDir, c.Path)
	existing, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("concept not found for update: %s", c.Path)
	}

	// Create version snapshot before overwriting
	if err := b.snapshot(c.Path, existing); err != nil {
		return fmt.Errorf("create version snapshot: %w", err)
	}

	return b.writeFile(c)
}

// History returns the timestamps of all version snapshots for a concept.
func (b *Bundle) History(path string) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	verPath := filepath.Join(b.verDir, path)
	dir := strings.TrimSuffix(verPath, ".md")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read version history: %w", err)
	}

	var timestamps []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			ts := strings.TrimSuffix(e.Name(), ".md")
			timestamps = append(timestamps, ts)
		}
	}
	sort.Strings(timestamps)
	return timestamps, nil
}

// Diff returns a unified diff between a version snapshot and the current file.
func (b *Bundle) Diff(path, timestamp string) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Read current
	currentData, err := os.ReadFile(filepath.Join(b.rootDir, path))
	if err != nil {
		return "", fmt.Errorf("read current %s: %w", path, err)
	}

	// Read version
	verPath := filepath.Join(b.verDir, path)
	verDir := strings.TrimSuffix(verPath, ".md")
	versionData, err := os.ReadFile(filepath.Join(verDir, timestamp+".md"))
	if err != nil {
		return "", fmt.Errorf("read version %s@%s: %w", path, timestamp, err)
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(string(versionData), string(currentData), true)
	diffs = dmp.DiffCleanupSemantic(diffs)
	return dmp.DiffPrettyText(diffs), nil
}

// RootDir returns the bundle root directory path.
func (b *Bundle) RootDir() string {
	return b.rootDir
}

func (b *Bundle) writeFile(c *Concept) error {
	data, err := SerializeConcept(c)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(b.rootDir, c.Path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	return os.WriteFile(fullPath, data, 0o644)
}

func (b *Bundle) snapshot(path string, data []byte) error {
	verPath := filepath.Join(b.verDir, path)
	verDir := strings.TrimSuffix(verPath, ".md")
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return err
	}

	ts := time.Now().UTC().Format("2006-01-02T15-04-05")
	return os.WriteFile(filepath.Join(verDir, ts+".md"), data, 0o644)
}
