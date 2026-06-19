package viz

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
)

//go:embed template/viz.html
var templateHTML string

var crossLinkRe = regexp.MustCompile(`\]\(([^)\s]+\.md)`)

type graphNode struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Resource    string   `json:"resource,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Timestamp   string   `json:"timestamp,omitempty"`
	Body        string   `json:"body,omitempty"`
}

type graphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type graphData struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

func newGraphData() graphData {
	return graphData{
		Nodes: []graphNode{},
		Edges: []graphEdge{},
	}
}

// Generate produces a self-contained viz.html from the given concepts.
func Generate(concepts []*bundle.Concept) (string, error) {
	// Build node index
	pathSet := make(map[string]bool, len(concepts))
	for _, c := range concepts {
		pathSet[c.Path] = true
	}

	data := newGraphData()

	for _, c := range concepts {
		data.Nodes = append(data.Nodes, graphNode{
			ID:          c.Path,
			Title:       c.Meta.Title,
			Type:        c.Meta.Type,
			Description: c.Meta.Description,
			Resource:    c.Meta.Resource,
			Tags:        c.Meta.Tags,
			Timestamp:   c.Meta.Timestamp,
			Body:        c.Body,
		})

		// Extract cross-links from body
		matches := crossLinkRe.FindAllStringSubmatch(c.Body, -1)
		for _, m := range matches {
			target := normalizePath(c.Path, m[1])
			if pathSet[target] && target != c.Path {
				data.Edges = append(data.Edges, graphEdge{
					Source: c.Path,
					Target: target,
				})
			}
		}
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal graph data: %w", err)
	}

	// Replace the data placeholder in the template
	result := strings.Replace(
		templateHTML,
		`/*DATA_JSON*/{"nodes":[],"edges":[]}/*END_DATA*/`,
		string(jsonBytes),
		1,
	)

	return result, nil
}

// normalizePath resolves a relative link target against the source file's directory.
func normalizePath(sourcePath, target string) string {
	// Absolute paths (starting with /)
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}

	// Relative paths — resolve against source directory
	parts := strings.Split(sourcePath, "/")
	if len(parts) > 1 {
		dir := strings.Join(parts[:len(parts)-1], "/")
		return dir + "/" + target
	}
	return target
}
