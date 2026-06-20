package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// ErrConflict is returned when an update's expected version doesn't match.
var ErrConflict = errors.New("version conflict")

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

// VerDir returns the versions directory path.
func (b *Bundle) VerDir() string {
	return b.verDir
}

// conceptVersion returns the current version of a concept by counting
// version-named snapshots in .versions/{dir}/{name}.{N}.md.
// Returns 1 if no snapshots exist. Must be called with lock held.
func (b *Bundle) conceptVersion(path string) int {
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	verDir := filepath.Join(b.verDir, filepath.Dir(path), name)

	entries, err := os.ReadDir(verDir)
	if err != nil {
		return 1
	}

	highest := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		// Parse {name}.{N}.md
		fname := strings.TrimSuffix(e.Name(), ".md")
		parts := strings.SplitN(fname, ".", 2)
		if len(parts) != 2 {
			continue
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		if n > highest {
			highest = n
		}
	}
	return highest + 1
}

// Read reads and parses a concept file by its relative path.
func (b *Bundle) Read(path string) (*Concept, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	data, err := os.ReadFile(filepath.Join(b.rootDir, path))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	c, err := ParseConcept(data, path)
	if err != nil {
		return nil, err
	}
	c.Version = b.conceptVersion(path)
	return c, nil
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
		c.Version = b.conceptVersion(rel)
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

	c.Version = 1
	return b.writeFile(c)
}

// Update overwrites an existing concept file, creating a version snapshot first.
// If expectedVersion > 0, the update is rejected with ErrConflict if the current
// version doesn't match. Pass 0 to skip the check (backwards compatibility).
func (b *Bundle) Update(c *Concept, expectedVersion int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	fullPath := filepath.Join(b.rootDir, c.Path)
	existing, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("concept not found for update: %s", c.Path)
	}

	currentVersion := b.conceptVersion(c.Path)

	if expectedVersion > 0 && expectedVersion != currentVersion {
		return fmt.Errorf("%w: expected version %d but current is %d", ErrConflict, expectedVersion, currentVersion)
	}

	// Create version snapshot before overwriting
	if err := b.snapshot(c.Path, existing, currentVersion); err != nil {
		return fmt.Errorf("create version snapshot: %w", err)
	}

	c.Version = currentVersion + 1
	return b.writeFile(c)
}

// SnapshotPath returns the relative path of a snapshot file within the versions dir.
// Useful for pushing snapshots to S3.
func SnapshotPath(conceptPath string, version int) string {
	name := strings.TrimSuffix(filepath.Base(conceptPath), ".md")
	dir := filepath.Join(filepath.Dir(conceptPath), name)
	return filepath.Join(dir, fmt.Sprintf("%s.%d.md", name, version))
}

// History returns the version numbers of all snapshots for a concept.
func (b *Bundle) History(path string) ([]int, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	name := strings.TrimSuffix(filepath.Base(path), ".md")
	verDir := filepath.Join(b.verDir, filepath.Dir(path), name)

	entries, err := os.ReadDir(verDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read version history: %w", err)
	}

	var versions []int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		fname := strings.TrimSuffix(e.Name(), ".md")
		parts := strings.SplitN(fname, ".", 2)
		if len(parts) != 2 {
			continue
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		versions = append(versions, n)
	}
	sort.Ints(versions)
	return versions, nil
}

// Diff returns a unified diff between a version snapshot and the current file.
func (b *Bundle) Diff(path string, version int) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Read current
	currentData, err := os.ReadFile(filepath.Join(b.rootDir, path))
	if err != nil {
		return "", fmt.Errorf("read current %s: %w", path, err)
	}

	// Read version snapshot
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	verDir := filepath.Join(b.verDir, filepath.Dir(path), name)
	versionData, err := os.ReadFile(filepath.Join(verDir, fmt.Sprintf("%s.%d.md", name, version)))
	if err != nil {
		return "", fmt.Errorf("read version %s@%d: %w", path, version, err)
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

func (b *Bundle) snapshot(path string, data []byte, version int) error {
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	verDir := filepath.Join(b.verDir, filepath.Dir(path), name)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s.%d.md", name, version)
	return os.WriteFile(filepath.Join(verDir, filename), data, 0o644)
}
