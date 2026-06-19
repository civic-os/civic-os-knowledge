package tools

import (
	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/civic-os/civic-os-knowledge/internal/search"
)

// Deps holds shared dependencies for all tool handlers.
type Deps struct {
	Bundle  *bundle.Bundle
	Index   *search.Index
	OnWrite func(path string) // called after create/update; noop if nil
}

func (d *Deps) onWrite(path string) {
	if d.OnWrite != nil {
		d.OnWrite(path)
	}
}
