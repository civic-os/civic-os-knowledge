package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MoveItem is one rename in a batch move.
type MoveItem struct {
	Path    string // current path
	NewPath string
	Version int // expected current version; 0 skips the check
}

// MovedConcept is a concept at its new path.
type MovedConcept struct {
	From    string
	Concept *Concept
	Carried int // version snapshots carried from the old path
}

// MoveResult describes everything a batch move changed.
type MoveResult struct {
	Moved    []MovedConcept
	Relinked []*Concept          // other concepts rewritten: links and/or stale aliases
	Mentions map[string][]string // old path -> concepts still naming it outside link syntax

	Written         []string // bundle-relative paths written
	Removed         []string // bundle-relative paths removed
	VersionsWritten []string // versions-relative paths written
	VersionsRemoved []string // versions-relative paths removed
}

// plannedWrite is a concept whose new content is computed before anything
// touches disk. The current bytes are snapshotted at version, and the file
// becomes version+1.
type plannedWrite struct {
	path    string
	before  []byte
	after   []byte
	version int
	concept *Concept
}

// MoveBatch renames concepts, carrying their version history, recording each
// old path in the concept's aliases, and rewriting links to them across the
// bundle. Each affected concept gets one new version. The batch is validated
// and computed in full before any file changes, and applied all-or-nothing.
func (b *Bundle) MoveBatch(items []MoveItem) (*MoveResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.validateMovesUnlocked(items); err != nil {
		return nil, err
	}
	moved, relinked, mentions, err := b.planMovesUnlocked(items)
	if err != nil {
		return nil, err
	}

	res := &MoveResult{Mentions: mentions}
	undoRenames, emptied, err := b.renameForMoveUnlocked(items, moved, res)
	if err != nil {
		return nil, err
	}

	writes := append(append([]plannedWrite{}, moved...), relinked...)
	for i, w := range writes {
		err := b.snapshot(w.path, w.before, w.version)
		if err == nil {
			err = b.writeRaw(w.path, w.after)
		}
		if err != nil {
			// Restore every write so far, then undo the renames.
			for _, done := range writes[:i+1] {
				_ = b.writeRaw(done.path, done.before)
				_ = os.Remove(filepath.Join(b.verDir, SnapshotPath(done.path, done.version)))
			}
			undoRenames()
			return nil, fmt.Errorf("move %s: %w", w.path, err)
		}
		res.VersionsWritten = append(res.VersionsWritten, filepath.ToSlash(SnapshotPath(w.path, w.version)))
		res.Written = append(res.Written, w.path)
	}

	for _, dir := range emptied {
		removeEmptyDirs(dir.root, dir.path)
	}
	for _, w := range relinked {
		res.Relinked = append(res.Relinked, w.concept)
	}
	return res, nil
}

func (b *Bundle) validateMovesUnlocked(items []MoveItem) error {
	if len(items) == 0 {
		return errors.New("no moves given")
	}
	srcs := make(map[string]bool)
	dsts := make(map[string]bool)
	for _, it := range items {
		if err := ValidatePath(it.NewPath); err != nil {
			return err
		}
		switch {
		case it.Path == it.NewPath:
			return fmt.Errorf("%s: new path is the same as the current path", it.Path)
		case srcs[it.Path]:
			return fmt.Errorf("%s is moved more than once in this batch", it.Path)
		case dsts[it.NewPath]:
			return fmt.Errorf("more than one concept is moved to %s", it.NewPath)
		}
		srcs[it.Path] = true
		dsts[it.NewPath] = true
	}
	for _, it := range items {
		if srcs[it.NewPath] {
			return fmt.Errorf("%s → %s: the target is also being moved in this batch; chains and swaps aren't supported, so split them into separate moves", it.Path, it.NewPath)
		}
		if !b.exists(it.Path) {
			if holder, ok := b.aliasHolderUnlocked(it.Path); ok {
				return &MovedError{Path: it.Path, Current: holder}
			}
			return fmt.Errorf("concept not found: %s", it.Path)
		}
		if b.exists(it.NewPath) {
			return fmt.Errorf("concept already exists: %s", it.NewPath)
		}
		if _, err := os.Stat(b.snapshotDir(it.NewPath)); err == nil {
			return fmt.Errorf("version history already exists for %s; refusing to merge it with %s's history", it.NewPath, it.Path)
		}
		if it.Version > 0 {
			if current := b.conceptVersion(it.Path); current != it.Version {
				return fmt.Errorf("%w: %s expected version %d but current is %d", ErrConflict, it.Path, it.Version, current)
			}
		}
	}
	return nil
}

// planMovesUnlocked computes the new content of every affected concept.
func (b *Bundle) planMovesUnlocked(items []MoveItem) (moved, relinked []plannedWrite, mentions map[string][]string, err error) {
	mapping := make(map[string]string, len(items))
	stale := make(map[string]bool, 2*len(items)) // aliases other concepts may no longer hold
	for _, it := range items {
		mapping[it.Path] = it.NewPath
		stale[it.Path] = true
		stale[it.NewPath] = true
	}
	oldPaths := make([]string, 0, len(items))
	for old := range mapping {
		oldPaths = append(oldPaths, old)
	}
	sort.Strings(oldPaths)

	retarget := func(l Link) (string, bool) {
		if np, ok := mapping[l.Resolved]; ok {
			return CanonicalTarget(np, l.Anchor), true
		}
		return "", false
	}

	paths, err := b.pathsUnlocked()
	if err != nil {
		return nil, nil, nil, err
	}
	exists := pathSet(paths)

	mentions = make(map[string][]string)
	recordMentions := func(c *Concept) {
		seen := make(map[string]bool)
		for _, text := range []string{c.Meta.Title, c.Meta.Description, c.Meta.Resource, c.Body} {
			for _, m := range FindMentions(text, c.Path, oldPaths) {
				if !seen[m.Path] {
					seen[m.Path] = true
					mentions[m.Path] = append(mentions[m.Path], c.Path)
				}
			}
		}
	}

	for _, it := range items {
		before, err := os.ReadFile(filepath.Join(b.rootDir, it.Path))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read %s: %w", it.Path, err)
		}
		c, err := ParseConcept(before, it.Path)
		if err != nil {
			return nil, nil, nil, err
		}
		// Retarget links to moved concepts, and pin any relative links to
		// existing concepts to canonical form since the directory may change.
		for _, field := range []*string{&c.Meta.Title, &c.Meta.Description, &c.Body} {
			*field, _ = RewriteLinks(*field, it.Path, func(l Link) (string, bool) {
				if t, ok := retarget(l); ok {
					return t, true
				}
				if exists[l.Resolved] {
					return CanonicalTarget(l.Resolved, l.Anchor), true
				}
				return "", false
			})
		}
		c.Meta.Aliases = withoutPath(appendUnique(c.Meta.Aliases, it.Path), it.NewPath)
		c.Path = it.NewPath
		after, err := SerializeConcept(c)
		if err != nil {
			return nil, nil, nil, err
		}
		version := b.conceptVersion(it.Path)
		c.Version = version + 1
		moved = append(moved, plannedWrite{path: it.NewPath, before: before, after: after, version: version, concept: c})
		recordMentions(c)
	}

	for _, p := range paths {
		if _, isMoved := mapping[p]; isMoved {
			continue
		}
		before, err := os.ReadFile(filepath.Join(b.rootDir, p))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read %s: %w", p, err)
		}
		// Rewrite the raw file so frontmatter formatting survives untouched.
		text, n := RewriteLinks(string(before), p, retarget)
		c, err := ParseConcept([]byte(text), p)
		if err != nil {
			continue // not a concept (e.g. an index.md without frontmatter)
		}
		changed := n > 0
		if kept := withoutPaths(c.Meta.Aliases, stale); len(kept) != len(c.Meta.Aliases) {
			c.Meta.Aliases = kept
			data, err := SerializeConcept(c)
			if err != nil {
				return nil, nil, nil, err
			}
			text, changed = string(data), true
		}
		recordMentions(c)
		if !changed {
			continue
		}
		version := b.conceptVersion(p)
		c.Version = version + 1
		relinked = append(relinked, plannedWrite{path: p, before: before, after: []byte(text), version: version, concept: c})
	}
	return moved, relinked, mentions, nil
}

type emptiedDir struct {
	root, path string
}

// renameForMoveUnlocked moves each concept file and its snapshots, renaming
// snapshots to the new name. On failure every rename is undone. It returns an
// undo func for later failures and the directories that may now be empty.
func (b *Bundle) renameForMoveUnlocked(items []MoveItem, moved []plannedWrite, res *MoveResult) (undo func(), emptied []emptiedDir, err error) {
	type rename struct{ from, to string }
	var done []rename
	undo = func() {
		for i := len(done) - 1; i >= 0; i-- {
			_ = os.Rename(done[i].to, done[i].from)
		}
	}
	mv := func(from, to string) error {
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
		done = append(done, rename{from, to})
		return nil
	}
	relVer := func(p string) string {
		r, _ := filepath.Rel(b.verDir, p)
		return filepath.ToSlash(r)
	}

	for i, it := range items {
		oldDir, newDir := b.snapshotDir(it.Path), b.snapshotDir(it.NewPath)
		newName := strings.TrimSuffix(filepath.Base(it.NewPath), ".md")
		entries, err := os.ReadDir(oldDir)
		if err != nil && !os.IsNotExist(err) {
			undo()
			return nil, nil, fmt.Errorf("read version history of %s: %w", it.Path, err)
		}
		carried := 0
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if n, ok := parseSnapshotVersion(name); ok {
				name = fmt.Sprintf("%s.%d.md", newName, n)
				carried++
			}
			from, to := filepath.Join(oldDir, e.Name()), filepath.Join(newDir, name)
			if err := mv(from, to); err != nil {
				undo()
				return nil, nil, fmt.Errorf("move version history of %s: %w", it.Path, err)
			}
			res.VersionsRemoved = append(res.VersionsRemoved, relVer(from))
			res.VersionsWritten = append(res.VersionsWritten, relVer(to))
		}
		emptied = append(emptied, emptiedDir{b.verDir, oldDir})

		from, to := filepath.Join(b.rootDir, it.Path), filepath.Join(b.rootDir, it.NewPath)
		if err := mv(from, to); err != nil {
			undo()
			return nil, nil, fmt.Errorf("move %s: %w", it.Path, err)
		}
		emptied = append(emptied, emptiedDir{b.rootDir, filepath.Dir(from)})
		res.Removed = append(res.Removed, it.Path)
		res.Moved = append(res.Moved, MovedConcept{From: it.Path, Concept: moved[i].concept, Carried: carried})
	}
	return undo, emptied, nil
}

// removeEmptyDirs removes dir and its parents while they are empty, stopping
// at root.
func removeEmptyDirs(root, dir string) {
	for dir != root && strings.HasPrefix(dir, root+string(filepath.Separator)) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func appendUnique(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}

func withoutPath(list []string, s string) []string {
	return withoutPaths(list, map[string]bool{s: true})
}

func withoutPaths(list []string, drop map[string]bool) []string {
	var out []string
	for _, v := range list {
		if !drop[v] {
			out = append(out, v)
		}
	}
	return out
}
