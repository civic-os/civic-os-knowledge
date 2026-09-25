package bundle

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	udiff "github.com/aymanbagabas/go-udiff"
)

// ErrConflict is returned when an update's expected version doesn't match.
var ErrConflict = errors.New("version conflict")

// MovedError is returned when a write targets a path that a concept has
// moved away from. Reads follow aliases; writes don't.
type MovedError struct {
	Path    string // path the caller asked for
	Current string // path the concept lives at now
}

func (e *MovedError) Error() string {
	return fmt.Sprintf("%s was moved to %s — use that path", e.Path, e.Current)
}

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

// RootDir returns the bundle root directory path.
func (b *Bundle) RootDir() string {
	return b.rootDir
}

// ValidatePath reports whether p is an acceptable concept path: a clean,
// relative, slash-separated path ending in .md with no empty or
// dot-prefixed segments (which also rules out "..").
func ValidatePath(p string) error {
	switch {
	case p == "":
		return errors.New("path is empty")
	case strings.HasPrefix(p, "/"):
		return fmt.Errorf("invalid path %q: must be relative (no leading slash)", p)
	case !strings.HasSuffix(p, ".md"):
		return fmt.Errorf("invalid path %q: must end in .md", p)
	case strings.Contains(p, `\`) || path.Clean(p) != p:
		return fmt.Errorf("invalid path %q: must be a clean slash-separated path", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || strings.HasPrefix(seg, ".") {
			return fmt.Errorf("invalid path %q: segments must not be empty or start with '.'", p)
		}
	}
	return nil
}

// parseSnapshotVersion extracts N from a snapshot filename "{name}.{N}.md".
// The name may itself contain dots.
func parseSnapshotVersion(filename string) (int, bool) {
	s, ok := strings.CutSuffix(filename, ".md")
	if !ok {
		return 0, false
	}
	i := strings.LastIndex(s, ".")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[i+1:])
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// snapshotDir returns the versions directory holding a concept's snapshots.
func (b *Bundle) snapshotDir(conceptPath string) string {
	name := strings.TrimSuffix(filepath.Base(conceptPath), ".md")
	return filepath.Join(b.verDir, filepath.Dir(conceptPath), name)
}

// snapshotVersions returns the sorted snapshot versions of a concept.
// Must be called with lock held.
func (b *Bundle) snapshotVersions(conceptPath string) ([]int, error) {
	entries, err := os.ReadDir(b.snapshotDir(conceptPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read version history: %w", err)
	}
	var versions []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if n, ok := parseSnapshotVersion(e.Name()); ok {
			versions = append(versions, n)
		}
	}
	sort.Ints(versions)
	return versions, nil
}

// conceptVersion returns the current version of a concept: one more than its
// highest snapshot, or 1 if it has none. Must be called with lock held.
func (b *Bundle) conceptVersion(conceptPath string) int {
	versions, _ := b.snapshotVersions(conceptPath)
	if len(versions) == 0 {
		return 1
	}
	return versions[len(versions)-1] + 1
}

// exists reports whether a concept file exists at p. Must be called with lock held.
func (b *Bundle) exists(p string) bool {
	info, err := os.Stat(filepath.Join(b.rootDir, p))
	return err == nil && !info.IsDir()
}

// Resolve returns the path a concept currently lives at. A file at p wins;
// otherwise the concept holding p as an alias is returned with viaAlias set.
// If neither exists, p is returned unchanged.
func (b *Bundle) Resolve(p string) (current string, viaAlias bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.resolveUnlocked(p)
}

func (b *Bundle) resolveUnlocked(p string) (string, bool) {
	if b.exists(p) {
		return p, false
	}
	if holder, ok := b.aliasHolderUnlocked(p); ok {
		return holder, true
	}
	return p, false
}

// aliasHolderUnlocked returns the concept that lists p as an alias.
func (b *Bundle) aliasHolderUnlocked(p string) (string, bool) {
	aliases, err := b.aliasMapUnlocked()
	if err != nil {
		return "", false
	}
	holder, ok := aliases[p]
	return holder, ok
}

// aliasMapUnlocked maps every alias path to the concept holding it. If two
// concepts claim the same alias (only possible with hand-edited files), the
// lexically first holder wins.
func (b *Bundle) aliasMapUnlocked() (map[string]string, error) {
	concepts, err := b.listUnlocked()
	if err != nil {
		return nil, err
	}
	aliases := make(map[string]string)
	for _, c := range concepts {
		for _, a := range c.Meta.Aliases {
			if _, taken := aliases[a]; !taken {
				aliases[a] = c.Path
			}
		}
	}
	return aliases, nil
}

// pathsUnlocked returns every concept file path in the bundle, sorted.
func (b *Bundle) pathsUnlocked() ([]string, error) {
	var paths []string
	err := filepath.Walk(b.rootDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		rel, err := filepath.Rel(b.rootDir, p)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk bundle: %w", err)
	}
	return paths, nil
}

// Read reads and parses a concept file by its relative path, following
// aliases. When an alias is followed, the returned concept's Path is the
// current path and ResolvedFrom is the requested one.
func (b *Bundle) Read(p string) (*Concept, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	current, viaAlias := b.resolveUnlocked(p)
	c, err := b.readUnlocked(current)
	if err != nil {
		return nil, err
	}
	if viaAlias {
		c.ResolvedFrom = p
	}
	return c, nil
}

func (b *Bundle) readUnlocked(p string) (*Concept, error) {
	data, err := os.ReadFile(filepath.Join(b.rootDir, p))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	c, err := ParseConcept(data, p)
	if err != nil {
		return nil, err
	}
	c.Version = b.conceptVersion(p)
	return c, nil
}

// Suggest returns up to three existing concept paths that a missing path
// may have meant: the alias holder, then same-name and prefix matches.
func (b *Bundle) Suggest(p string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	paths, err := b.pathsUnlocked()
	if err != nil {
		return nil
	}
	aliases, _ := b.aliasMapUnlocked()
	return suggest(p, p, paths, pathSet(paths), aliases)
}

// List returns all concept files in the bundle directory (recursively).
func (b *Bundle) List() ([]*Concept, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.listUnlocked()
}

func (b *Bundle) listUnlocked() ([]*Concept, error) {
	paths, err := b.pathsUnlocked()
	if err != nil {
		return nil, fmt.Errorf("list bundle: %w", err)
	}
	var concepts []*Concept
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Join(b.rootDir, p))
		if err != nil {
			return nil, fmt.Errorf("list bundle: %w", err)
		}
		c, err := ParseConcept(data, p)
		if err != nil {
			// Skip files that fail to parse (e.g., index.md without frontmatter)
			continue
		}
		c.Version = b.conceptVersion(p)
		concepts = append(concepts, c)
	}
	return concepts, nil
}

// Create writes a new concept file. Returns an error if the file already
// exists, the path is invalid, or the concept would introduce broken links.
// Concept links are normalized to canonical form before writing.
func (b *Bundle) Create(c *Concept) (*LinkReport, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := ValidatePath(c.Path); err != nil {
		return nil, err
	}
	if b.exists(c.Path) {
		return nil, fmt.Errorf("concept already exists: %s", c.Path)
	}

	report, err := b.checkLinksUnlocked(c, nil)
	if err != nil {
		return report, err
	}

	c.Version = 1
	return report, b.writeFile(c)
}

// Update overwrites an existing concept file, creating a version snapshot first.
// If expectedVersion > 0, the update is rejected with ErrConflict if the current
// version doesn't match. Pass 0 to skip the check (backwards compatibility).
// Updating a path the concept has moved away from returns *MovedError.
func (b *Bundle) Update(c *Concept, expectedVersion int) (*LinkReport, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	fullPath := filepath.Join(b.rootDir, c.Path)
	existing, err := os.ReadFile(fullPath)
	if err != nil {
		if holder, ok := b.aliasHolderUnlocked(c.Path); ok {
			return nil, &MovedError{Path: c.Path, Current: holder}
		}
		return nil, fmt.Errorf("concept not found for update: %s", c.Path)
	}

	currentVersion := b.conceptVersion(c.Path)

	if expectedVersion > 0 && expectedVersion != currentVersion {
		return nil, fmt.Errorf("%w: expected version %d but current is %d", ErrConflict, expectedVersion, currentVersion)
	}

	prev, _ := ParseConcept(existing, c.Path) // nil prev treats every broken link as new
	report, err := b.checkLinksUnlocked(c, prev)
	if err != nil {
		return report, err
	}

	// Create version snapshot before overwriting
	if err := b.snapshot(c.Path, existing, currentVersion); err != nil {
		return nil, fmt.Errorf("create version snapshot: %w", err)
	}

	c.Version = currentVersion + 1
	return report, b.writeFile(c)
}

// SnapshotPath returns the relative path of a snapshot file within the versions dir.
// Useful for pushing snapshots to S3.
func SnapshotPath(conceptPath string, version int) string {
	name := strings.TrimSuffix(filepath.Base(conceptPath), ".md")
	dir := filepath.Join(filepath.Dir(conceptPath), name)
	return filepath.Join(dir, fmt.Sprintf("%s.%d.md", name, version))
}

// History returns the version numbers of all snapshots for a concept,
// following aliases.
func (b *Bundle) History(p string) ([]int, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	current, _ := b.resolveUnlocked(p)
	return b.snapshotVersions(current)
}

// Diff returns a unified diff between a version snapshot and the current
// file, following aliases. Returns "" if they are identical.
func (b *Bundle) Diff(p string, version int) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	current, _ := b.resolveUnlocked(p)
	currentData, err := os.ReadFile(filepath.Join(b.rootDir, current))
	if err != nil {
		return "", fmt.Errorf("read current %s: %w", current, err)
	}

	versionData, err := os.ReadFile(filepath.Join(b.verDir, SnapshotPath(current, version)))
	if err != nil {
		return "", fmt.Errorf("read version %s@%d: %w", current, version, err)
	}

	return udiff.Unified(
		fmt.Sprintf("%s@v%d", current, version),
		current+" (current)",
		string(versionData),
		string(currentData),
	), nil
}

func (b *Bundle) writeFile(c *Concept) error {
	data, err := SerializeConcept(c)
	if err != nil {
		return err
	}
	return b.writeRaw(c.Path, data)
}

func (b *Bundle) writeRaw(p string, data []byte) error {
	fullPath := filepath.Join(b.rootDir, p)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	return os.WriteFile(fullPath, data, 0o644)
}

func (b *Bundle) snapshot(conceptPath string, data []byte, version int) error {
	full := filepath.Join(b.verDir, SnapshotPath(conceptPath, version))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func pathSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	return set
}
