package bundle

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
	"gopkg.in/yaml.v3"
)

// ConceptMeta represents the YAML frontmatter of an OKF concept file.
type ConceptMeta struct {
	Type        string   `yaml:"type"`
	Title       string   `yaml:"title,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Resource    string   `yaml:"resource,omitempty"`
	Tags        []string `yaml:"tags,omitempty,flow"`
	Timestamp   string   `yaml:"timestamp,omitempty"`
}

// Concept represents a complete OKF concept file: frontmatter + body.
type Concept struct {
	Meta    ConceptMeta
	Body    string
	Path    string // relative path within the bundle
	Version int    // derived from snapshot count, not stored in YAML
}

// ParseConcept parses a markdown file with YAML frontmatter into a Concept.
func ParseConcept(data []byte, path string) (*Concept, error) {
	var meta ConceptMeta
	body, err := frontmatter.Parse(bytes.NewReader(data), &meta)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter for %s: %w", path, err)
	}
	if meta.Type == "" {
		return nil, fmt.Errorf("missing required field 'type' in %s", path)
	}
	return &Concept{
		Meta: meta,
		Body: strings.TrimSpace(string(body)),
		Path: path,
	}, nil
}

// SerializeConcept serializes a Concept back to markdown with YAML frontmatter.
func SerializeConcept(c *Concept) ([]byte, error) {
	fm, err := yaml.Marshal(&c.Meta)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fm)
	buf.WriteString("---\n")
	if c.Body != "" {
		buf.WriteString("\n")
		buf.WriteString(c.Body)
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

// NowTimestamp returns the current date as a YYYY-MM-DD string.
func NowTimestamp() string {
	return time.Now().UTC().Format("2006-01-02")
}
