package bundle

import (
	"fmt"
	"path"
	"strings"
)

// ConceptType is a registered concept type. Each type owns one top-level
// folder, and a concept's folder decides its type: the frontmatter `type`
// (required by OKF) always matches the registry entry for the folder.
type ConceptType struct {
	Name   string // frontmatter type, e.g. "Client Profile"
	Folder string // top-level folder, e.g. "clients"
	Holds  string // what belongs in the folder
}

// Types is the registry of concept types, one per top-level folder. It is
// closed and hard-coded: adding a type is a code change, so the set stays
// deliberate. (If it ever needs to vary per deployment, load it at startup,
// e.g. from an environment variable, and keep this as the default.)
var Types = []ConceptType{
	{"Client Profile", "clients", "Clients and relationship status"},
	{"Company Reference", "company", "Company facts: registration, address, financials, people"},
	{"Competitive Analysis", "competitive-analysis", "Competitors and how Civic OS compares"},
	{"Decision Record", "decisions", "Architecture and business decisions"},
	{"Infrastructure Component", "infrastructure", "Servers, clusters and shared services"},
	{"Instance Deployment", "instances", "Per-instance deployment status and configuration"},
	{"Marketing Document", "marketing", "Brand, messaging, campaigns and communication conventions"},
	{"Meeting Note", "meeting-notes", "Meeting notes, decisions and action items"},
	{"Partner Profile", "partners", "Partner organizations and programs"},
	{"Project Specification", "projects", "Project requirements and specifications"},
	{"Proposal", "proposals", "Sales proposals and grant applications"},
	{"Prospect", "prospects", "Potential clients and leads"},
	{"Research Analysis", "research", "Market, technical and ecosystem research"},
	{"Runbook", "runbooks", "Repeatable operational procedures"},
	{"Strategy Document", "strategy", "Business strategy, pricing and positioning"},
	{"Tool Reference", "tools", "Third-party tools and how they're used"},
}

// TopFolder returns the first segment of a concept path, or "" for a
// root-level path.
func TopFolder(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

// TypeForFolder returns the type registered for a top-level folder.
func TypeForFolder(folder string) (ConceptType, bool) {
	for _, t := range Types {
		if t.Folder == folder {
			return t, true
		}
	}
	return ConceptType{}, false
}

// TypeForPath returns the type registered for the folder a concept path is in.
func TypeForPath(p string) (ConceptType, bool) {
	return TypeForFolder(TopFolder(p))
}

// TypeByName returns the registered type with the given name, ignoring case.
func TypeByName(name string) (ConceptType, bool) {
	for _, t := range Types {
		if strings.EqualFold(t.Name, strings.TrimSpace(name)) {
			return t, true
		}
	}
	return ConceptType{}, false
}

// ResolveType returns the type a concept at p must have. requested may be
// empty (the folder's type is used) or any casing of that type. It errors if
// p isn't in a registered folder or requested belongs to another folder.
func ResolveType(p, requested string) (string, error) {
	folder := TopFolder(p)
	reg, ok := TypeForFolder(folder)
	if !ok {
		return "", fmt.Errorf("%s is not in a registered folder; concepts live in %s", p, folderList())
	}
	if requested == "" || strings.EqualFold(strings.TrimSpace(requested), reg.Name) {
		return reg.Name, nil
	}
	if t, ok := TypeByName(requested); ok {
		return "", fmt.Errorf("%s/ holds %s concepts; %s concepts live in %s/ (e.g. %s/%s)",
			folder, reg.Name, t.Name, t.Folder, t.Folder, path.Base(p))
	}
	return "", fmt.Errorf("unknown type %q; %s/ holds %s concepts (the type follows the folder)", requested, folder, reg.Name)
}

// TypeProblem describes how a stored concept breaks the folder/type rule, or
// returns "" if it follows it. Concepts written before the registry existed
// may not; writes warn about them rather than failing.
func TypeProblem(c *Concept) string {
	reg, ok := TypeForPath(c.Path)
	if !ok {
		hint := ""
		if t, ok := TypeByName(c.Meta.Type); ok {
			hint = fmt.Sprintf("; move it to %s/%s", t.Folder, path.Base(c.Path))
		}
		return fmt.Sprintf("%s is outside the registered folders%s", c.Path, hint)
	}
	if c.Meta.Type != reg.Name {
		return fmt.Sprintf("%s has type %q but %s/ holds %s concepts", c.Path, c.Meta.Type, reg.Folder, reg.Name)
	}
	return ""
}

func folderList() string {
	folders := make([]string, len(Types))
	for i, t := range Types {
		folders[i] = t.Folder + "/"
	}
	return strings.Join(folders, ", ")
}
