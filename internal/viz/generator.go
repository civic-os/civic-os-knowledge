package viz

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
)

//go:embed template/viz.html
var templateHTML string

// palette colors registered types that don't set their own. Gray is left out:
// the template draws unregistered types gray.
var palette = []string{
	"#f97316", "#0ea5e9", "#f43f5e", "#8b5cf6", "#ef4444", "#14b8a6",
	"#84cc16", "#6366f1", "#d946ef", "#3b82f6", "#ec4899", "#facc15",
	"#06b6d4", "#10b981", "#f59e0b", "#22c55e", "#a855f7", "#e11d48",
}

// typeColors gives each registered type a node color: its configured color,
// or a palette color picked by a hash of its name, so it stays put when the
// registry is reordered or extended.
func typeColors(types *bundle.Registry) map[string]string {
	colors := make(map[string]string)
	for _, t := range types.Types() {
		if t.Color != "" {
			colors[t.Name] = t.Color
			continue
		}
		h := fnv.New32a()
		h.Write([]byte(t.Name))
		colors[t.Name] = palette[h.Sum32()%uint32(len(palette))]
	}
	return colors
}

type graphNode struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Resource    string   `json:"resource,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Timestamp   string   `json:"timestamp,omitempty"`
	Body        string   `json:"body,omitempty"`
	Status      string   `json:"status,omitempty"`
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

// Generate produces a self-contained viz.html from the given concepts, coloring
// nodes by the registered types.
func Generate(concepts []*bundle.Concept, types *bundle.Registry) (string, error) {
	// Build node index
	pathSet := make(map[string]bool, len(concepts))
	for _, c := range concepts {
		pathSet[c.Path] = true
	}

	data := newGraphData()
	edgeSeen := make(map[string]bool)

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
			Status:      c.Meta.Status,
		})

		// Edges follow the same link model kb_move rewrites
		for _, target := range bundle.ConceptLinks(c) {
			edgeKey := c.Path + "\x00" + target
			if pathSet[target] && target != c.Path && !edgeSeen[edgeKey] {
				edgeSeen[edgeKey] = true
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

	colorBytes, err := json.Marshal(typeColors(types))
	if err != nil {
		return "", fmt.Errorf("marshal type colors: %w", err)
	}

	// Replace the placeholders in the template
	result := strings.NewReplacer(
		`/*DATA_JSON*/{"nodes":[],"edges":[]}/*END_DATA*/`, string(jsonBytes),
		`/*TYPE_COLORS*/{}/*END_TYPE_COLORS*/`, string(colorBytes),
	).Replace(templateHTML)

	return result, nil
}
