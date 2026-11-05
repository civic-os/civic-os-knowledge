package bundle

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// ConceptType is a registered concept type. Each type owns one top-level
// folder, and a concept's folder decides its type: the frontmatter `type`
// (required by OKF) always matches the registry entry for the folder.
type ConceptType struct {
	Name   string `json:"name"`            // frontmatter type, e.g. "Client Profile"
	Folder string `json:"folder"`          // top-level folder, e.g. "clients"
	Holds  string `json:"holds"`           // what belongs in the folder
	Color  string `json:"color,omitempty"` // viz node color (#rrggbb); optional
}

// Registry is the set of concept types a deployment accepts, one per
// top-level folder. It is loaded once at startup (KB_TYPES, or the embedded
// defaults) and is read-only afterwards.
type Registry struct {
	types []ConceptType
}

//go:embed default_types.json
var defaultTypesJSON []byte

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// NewRegistry validates types and returns a registry over them. Names must be
// unique (ignoring case), folders unique single path segments, and every type
// needs a description.
func NewRegistry(types []ConceptType) (*Registry, error) {
	if len(types) == 0 {
		return nil, errors.New("type registry is empty")
	}
	types = append([]ConceptType(nil), types...) // trimmed below; don't touch the caller's slice
	names := make(map[string]bool, len(types))
	folders := make(map[string]bool, len(types))
	var problems []string
	for i, t := range types {
		t.Name = strings.TrimSpace(t.Name)
		t.Folder = strings.TrimSpace(t.Folder)
		t.Holds = strings.TrimSpace(t.Holds)
		types[i] = t
		label := fmt.Sprintf("type %d (%q)", i+1, t.Name)
		switch {
		case t.Name == "":
			problems = append(problems, fmt.Sprintf("type %d has no name", i+1))
		case names[strings.ToLower(t.Name)]:
			problems = append(problems, fmt.Sprintf("%s: duplicate name", label))
		}
		names[strings.ToLower(t.Name)] = true
		switch {
		case t.Folder == "" || strings.Contains(t.Folder, "/") || ValidatePath(t.Folder+"/x.md") != nil:
			problems = append(problems, fmt.Sprintf("%s: folder %q is not a valid top-level folder", label, t.Folder))
		case folders[t.Folder]:
			problems = append(problems, fmt.Sprintf("%s: folder %q is already registered", label, t.Folder))
		}
		folders[t.Folder] = true
		if t.Holds == "" {
			problems = append(problems, fmt.Sprintf("%s: holds is empty", label))
		}
		if t.Color != "" && !colorPattern.MatchString(t.Color) {
			problems = append(problems, fmt.Sprintf("%s: color %q is not #rrggbb", label, t.Color))
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid type registry: %s", strings.Join(problems, "; "))
	}
	return &Registry{types: types}, nil
}

// ParseRegistry reads a registry from a JSON array of types, as given in
// KB_TYPES. Unknown fields are rejected so typos don't pass silently.
func ParseRegistry(data []byte) (*Registry, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var types []ConceptType
	if err := dec.Decode(&types); err != nil {
		return nil, fmt.Errorf("parse type registry: %w", err)
	}
	if dec.More() {
		return nil, errors.New("parse type registry: trailing data after the JSON array")
	}
	return NewRegistry(types)
}

// DefaultRegistry returns the generic types used when a deployment doesn't
// configure its own.
func DefaultRegistry() *Registry {
	r, err := ParseRegistry(defaultTypesJSON)
	if err != nil {
		panic("embedded default_types.json: " + err.Error())
	}
	return r
}

// Types returns the registered types in registry order.
func (r *Registry) Types() []ConceptType {
	return append([]ConceptType(nil), r.types...)
}

// TopFolder returns the first segment of a concept path, or "" for a
// root-level path.
func TopFolder(p string) string {
	if folder, _, ok := strings.Cut(p, "/"); ok {
		return folder
	}
	return ""
}

// ForFolder returns the type registered for a top-level folder.
func (r *Registry) ForFolder(folder string) (ConceptType, bool) {
	for _, t := range r.types {
		if t.Folder == folder {
			return t, true
		}
	}
	return ConceptType{}, false
}

// ForPath returns the type registered for the folder a concept path is in.
func (r *Registry) ForPath(p string) (ConceptType, bool) {
	return r.ForFolder(TopFolder(p))
}

// ByName returns the registered type with the given name, ignoring case.
func (r *Registry) ByName(name string) (ConceptType, bool) {
	for _, t := range r.types {
		if strings.EqualFold(t.Name, strings.TrimSpace(name)) {
			return t, true
		}
	}
	return ConceptType{}, false
}

// Resolve returns the type a concept at p must have. requested may be empty
// (the folder's type is used) or any casing of that type. It errors if p
// isn't in a registered folder or requested belongs to another folder.
func (r *Registry) Resolve(p, requested string) (string, error) {
	folder := TopFolder(p)
	reg, ok := r.ForFolder(folder)
	if !ok {
		return "", fmt.Errorf("%s is not in a registered folder; concepts live in %s", p, r.folderList())
	}
	if requested == "" || strings.EqualFold(strings.TrimSpace(requested), reg.Name) {
		return reg.Name, nil
	}
	if t, ok := r.ByName(requested); ok {
		return "", fmt.Errorf("%s/ holds %s concepts; %s concepts live in %s/ (e.g. %s/%s)",
			folder, reg.Name, t.Name, t.Folder, t.Folder, path.Base(p))
	}
	return "", fmt.Errorf("unknown type %q; %s/ holds %s concepts (the type follows the folder)", requested, folder, reg.Name)
}

// Problem describes how a stored concept breaks the folder/type rule, or
// returns "" if it follows it. Concepts written before the registry existed,
// or in a folder the registry no longer lists, may not; writes warn about
// them rather than failing.
func (r *Registry) Problem(c *Concept) string {
	reg, ok := r.ForPath(c.Path)
	if !ok {
		hint := ""
		if t, ok := r.ByName(c.Meta.Type); ok {
			hint = fmt.Sprintf("; move it to %s/%s", t.Folder, path.Base(c.Path))
		}
		return fmt.Sprintf("%s is outside the registered folders%s", c.Path, hint)
	}
	if c.Meta.Type != reg.Name {
		return fmt.Sprintf("%s has type %q but %s/ holds %s concepts", c.Path, c.Meta.Type, reg.Folder, reg.Name)
	}
	return ""
}

func (r *Registry) folderList() string {
	folders := make([]string, len(r.types))
	for i, t := range r.types {
		folders[i] = t.Folder + "/"
	}
	return strings.Join(folders, ", ")
}
