// Package sonda builds and starts a program from a working copy and reads what
// it writes.
//
// A Target says how a program is built and run. A Run is one execution of it,
// start to exit, holding every line it wrote as a Log. Two runs of the same
// target, one at the revision under review and one at the revision it is
// measured against, are what a comparison is made from; the package runs one
// at a time and leaves the pairing to the caller.
package sonda

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// File is the name of the file targets are read from. It sits in the directory
// peneira was opened on, committed beside the code it describes.
const File = "peneira.toml"

// Target is one program in the repository that can be built and run.
type Target struct {
	Name string `toml:"name"`

	// Dir is where both commands run. In the file it is relative to the file;
	// Load makes it absolute.
	Dir string `toml:"dir"`

	// Build and Run are argument lists, not shell strings, so nothing depends
	// on a shell being present or on quoting rules. Build may be empty: an
	// interpreted program, or a `go run` style command, builds as it starts.
	Build []string `toml:"build"`
	Run   []string `toml:"run"`

	// Env is added to the environment the commands inherit.
	Env map[string]string `toml:"env"`
}

type file struct {
	Targets []Target `toml:"target"`
}

// Load reads the targets declared in dir. A directory that declares nothing
// yields no targets and no error; a file that cannot be read as targets is
// reported with the file's name, since the file is what has to be fixed.
func Load(dir string) ([]Target, error) {
	var f file
	md, err := toml.DecodeFile(filepath.Join(dir, File), &f)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		return nil, fmt.Errorf("%s: unknown key %s", File, keys[0])
	}
	for i := range f.Targets {
		t := &f.Targets[i]
		if t.Name == "" {
			return nil, fmt.Errorf("%s: target %d has no name", File, i+1)
		}
		if len(t.Run) == 0 {
			return nil, fmt.Errorf("%s: target %q has no run command", File, t.Name)
		}
		if t.Dir == "" {
			t.Dir = "."
		}
		if !filepath.IsAbs(t.Dir) {
			t.Dir = filepath.Join(dir, t.Dir)
		}
		t.Dir = filepath.Clean(t.Dir)
	}
	return f.Targets, nil
}

// Relocate returns the target as it stands in another working copy of the
// same repository: its directory is the same path under to as it was under
// root.
func (t Target) Relocate(root, to string) (Target, error) {
	rel, err := filepath.Rel(resolved(root), resolved(t.Dir))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return t, fmt.Errorf("target %q runs in %s, outside the repository at %s", t.Name, t.Dir, root)
	}
	t.Dir = filepath.Join(to, rel)
	return t, nil
}

// resolved follows symbolic links, so that a path a tool reported and a path
// a person typed compare equal when they name the same directory.
func resolved(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// environ is the process environment with the target's own on top of it.
func (t Target) environ() []string {
	env := os.Environ()
	for k, v := range t.Env {
		env = append(env, k+"="+v)
	}
	return env
}
