package state

import (
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Recent is the list of repositories opened before, kept so the application
// has something to offer when it starts without one.
type Recent struct {
	Path     string    `json:"path"`
	LastUsed time.Time `json:"last_used"`
}

// Name is the last element of the path, which is how a repository is usually
// referred to.
func (r Recent) Name() string { return filepath.Base(r.Path) }

const maxRecent = 12

// LoadRecent returns the recently opened repositories, most recent first.
// Directories that have since been moved or deleted are dropped.
func LoadRecent() []Recent {
	var list []Recent
	readJSON("recent.json", &list)
	kept := list[:0]
	for _, r := range list {
		if info, err := os.Stat(r.Path); err == nil && info.IsDir() {
			kept = append(kept, r)
		}
	}
	slices.SortFunc(kept, func(a, b Recent) int { return b.LastUsed.Compare(a.LastUsed) })
	return kept
}

// RememberRecent records that a repository was opened and returns the updated
// list.
func RememberRecent(path string) []Recent {
	list := LoadRecent()
	out := make([]Recent, 0, len(list)+1)
	out = append(out, Recent{Path: path, LastUsed: time.Now()})
	for _, r := range list {
		if r.Path != path {
			out = append(out, r)
		}
	}
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	writeJSON("recent.json", out)
	return out
}
