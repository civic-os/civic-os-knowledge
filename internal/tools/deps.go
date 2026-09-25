package tools

import (
	"github.com/civic-os/civic-os-knowledge/internal/bundle"
	"github.com/civic-os/civic-os-knowledge/internal/search"
)

// Deps holds shared dependencies for all tool handlers.
type Deps struct {
	Bundle           *bundle.Bundle
	Index            *search.Index
	OnWrite          func(path string)            // called after create/update/move writes; noop if nil
	OnSnapshot       func(snapshotRelPath string) // called after a snapshot is written; noop if nil
	OnDelete         func(path string)            // called after a concept file is removed by a move; noop if nil
	OnSnapshotDelete func(snapshotRelPath string) // called after a snapshot is removed by a move; noop if nil
}

func (d *Deps) onWrite(path string) {
	if d.OnWrite != nil {
		d.OnWrite(path)
	}
}

func (d *Deps) onSnapshot(snapshotRelPath string) {
	if d.OnSnapshot != nil {
		d.OnSnapshot(snapshotRelPath)
	}
}

func (d *Deps) onDelete(path string) {
	if d.OnDelete != nil {
		d.OnDelete(path)
	}
}

func (d *Deps) onSnapshotDelete(snapshotRelPath string) {
	if d.OnSnapshotDelete != nil {
		d.OnSnapshotDelete(snapshotRelPath)
	}
}
