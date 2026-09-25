package viz

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/civic-os/civic-os-knowledge/internal/bundle"
)

//go:embed template/viz.html
var templateHTML string

// typeColors gives each registered concept type a node color. Unregistered
// types fall back to gray in the template.
var typeColors = map[string]string{
	"Client Profile":           "#f97316",
	"Company Reference":        "#0ea5e9",
	"Competitive Analysis":     "#f43f5e",
	"Decision Record":          "#8b5cf6",
	"Infrastructure Component": "#ef4444",
	"Instance Deployment":      "#14b8a6",
	"Marketing Document":       "#84cc16",
	"Meeting Note":             "#6366f1",
	"Partner Profile":          "#d946ef",
	"Project Specification":    "#3b82f6",
	"Proposal":                 "#ec4899",
	"Prospect":                 "#facc15",
	"Research Analysis":        "#06b6d4",
	"Runbook":                  "#10b981",
	"Strategy Document":        "#f59e0b",
	"Tool Reference":           "#a8a29e",
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

// Generate produces a self-contained viz.html from the given concepts.
func Generate(concepts []*bundle.Concept) (string, error) {
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

	colorBytes, err := json.Marshal(typeColors)
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
