package git

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/chromafish/peneira/internal/proc"
	"github.com/chromafish/peneira/internal/vcs"
)

// diffArgs asks git for a diff of the given spec, with whatever output flags
// the caller adds. Rename detection is always on, so a moved file reads as a
// move rather than as a deletion and an unrelated addition.
func diffArgs(s vcs.DiffSpec) []string {
	switch {
	case s.Kind == vcs.DiffRange:
		return []string{"diff", "-M", "-C", s.From, s.To}
	case s.Rev == workingTree:
		return []string{"diff", "-M", "-C", "HEAD"}
	default:
		// --first-parent -m gives a merge a diff against the parent it was
		// merged into; for an ordinary commit it changes nothing.
		return []string{"show", "--format=", "-M", "-C", "--first-parent", "-m", s.Rev}
	}
}

// Files lists what a diff touches, with per-file line counts. The names and
// the counts are two passes, because git prints them in two different shapes
// and asking for both at once interleaves them.
func (r *Repo) Files(ctx context.Context, spec vcs.DiffSpec) ([]vcs.FileChange, error) {
	if spec.Empty() {
		return nil, nil
	}
	names, err := r.read(ctx, append(diffArgs(spec), "--name-status", "-z")...)
	if err != nil {
		return nil, err
	}
	files := parseNameStatus(string(names))

	// The counts are worth having but not worth failing over: without them the
	// list still says which files changed.
	if stat, err := r.read(ctx, append(diffArgs(spec), "--numstat", "-z")...); err == nil {
		counts := parseNumstat(string(stat))
		for i := range files {
			if c, ok := counts[files[i].Path]; ok {
				files[i].Added, files[i].Removed, files[i].Binary = c.added, c.removed, c.binary
			}
		}
	}
	return files, nil
}

// parseNameStatus reads the NUL-separated form of --name-status. Ordinary
// changes are a status and a path; a rename or a copy is a status with a score
// followed by two paths.
func parseNameStatus(out string) []vcs.FileChange {
	fields := splitNUL(out)
	var files []vcs.FileChange
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if status == "" || i >= len(fields) {
			break
		}
		fc := vcs.FileChange{Status: vcs.Status(status[0])}
		switch status[0] {
		case 'R', 'C':
			if i+1 >= len(fields) {
				return files
			}
			fc.OldPath, fc.Path = fields[i], fields[i+1]
			i += 2
		default:
			fc.Path = fields[i]
			i++
		}
		files = append(files, fc)
	}
	return files
}

type counts struct {
	added, removed int
	binary         bool
}

// parseNumstat reads the NUL-separated form of --numstat, whose records are
// "added TAB removed TAB path" for an ordinary change and "added TAB removed
// TAB" followed by two paths for a rename. A binary file has dashes where the
// numbers would be.
func parseNumstat(out string) map[string]counts {
	fields := splitNUL(out)
	stats := map[string]counts{}
	for i := 0; i < len(fields); {
		rec := fields[i]
		i++
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		var c counts
		if parts[0] == "-" || parts[1] == "-" {
			c.binary = true
		} else {
			c.added, _ = strconv.Atoi(parts[0])
			c.removed, _ = strconv.Atoi(parts[1])
		}
		path := parts[2]
		if path == "" {
			// A rename: the path is empty and the two names follow.
			if i+1 >= len(fields) {
				break
			}
			path = fields[i+1]
			i += 2
		}
		stats[path] = c
	}
	return stats
}

// splitNUL splits NUL-separated output, dropping the empty tail.
func splitNUL(out string) []string {
	out = strings.TrimSuffix(out, "\x00")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\x00")
}

// DiffStream returns the whole diff in git's unified format, as git writes it.
func (r *Repo) DiffStream(ctx context.Context, spec vcs.DiffSpec, context int) (io.ReadCloser, error) {
	if spec.Empty() {
		return io.NopCloser(strings.NewReader("")), nil
	}
	return proc.Stream(ctx, r.root, r.bin, append(diffArgs(spec), "--patch", "--unified="+strconv.Itoa(context))...)
}

// FileDiff returns one file's diff. The pathspec goes after a double dash, so
// a path that looks like a revision is still read as a path.
func (r *Repo) FileDiff(ctx context.Context, spec vcs.DiffSpec, path string, context int) (string, error) {
	if spec.Empty() {
		return "", nil
	}
	args := append(diffArgs(spec), "--patch", "--unified="+strconv.Itoa(context), "--", path)
	out, err := r.read(ctx, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FileContent returns one side of a file in full, used to expand the context
// around a hunk.
//
// The new side of the working tree is the file on disk: it is not in any
// object, which is the whole point of it.
func (r *Repo) FileContent(ctx context.Context, spec vcs.DiffSpec, path string, side vcs.Side) ([]byte, error) {
	before := side == vcs.Before
	rev := spec.Rev
	switch {
	case spec.Kind == vcs.DiffRange && before:
		rev = spec.From
	case spec.Kind == vcs.DiffRange:
		rev = spec.To
	case spec.Rev == workingTree && before:
		rev = "HEAD"
	case spec.Rev == workingTree:
		return os.ReadFile(r.path(path))
	case before:
		// A root commit has no parent, and nothing to show for the old side.
		rev = spec.Rev + "^"
	}
	return r.read(ctx, "show", rev+":"+path)
}
