package jj

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/chromafish/peneira/internal/vcs"
)

// args is how jj is asked for a diff.
func args(s vcs.DiffSpec) []string {
	switch s.Kind {
	case vcs.DiffRange:
		return []string{"diff", "--from", s.From, "--to", s.To}
	default:
		return []string{"diff", "-r", s.Rev}
	}
}

// Files lists the files a diff touches, with per-file line counts.
//
// This deliberately avoids fetching the diff body: on a large change the file
// list must appear immediately, and hunks are fetched per file on demand.
func (r *Repo) Files(ctx context.Context, spec vcs.DiffSpec) ([]vcs.FileChange, error) {
	if spec.Empty() {
		return nil, nil
	}
	summary, err := r.read(ctx, append(args(spec), "--summary")...)
	if err != nil {
		return nil, err
	}
	files := parseSummary(string(summary))

	// --stat is a second cheap pass; without it the file list has no sense of
	// scale. A failure here is not worth failing the whole diff over.
	if stat, err := r.read(ctx, append(args(spec), "--stat")...); err == nil {
		counts := parseStat(string(stat))
		for i := range files {
			if c, ok := counts[files[i].Path]; ok {
				files[i].Added, files[i].Removed = c[0], c[1]
			}
		}
	}
	return files, nil
}

// parseSummary reads `jj diff --summary` output: a status letter, a space, and
// a path, where renames and copies use git's "{old => new}" brace form.
func parseSummary(out string) []vcs.FileChange {
	var files []vcs.FileChange
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 || line[1] != ' ' {
			continue
		}
		fc := vcs.FileChange{Status: vcs.Status(line[0])}
		fc.OldPath, fc.Path = expandBraces(line[2:])
		files = append(files, fc)
	}
	return files
}

// parseStat reads `jj diff --stat` output, whose lines look like
// "main.go | 4 ++--", where the number is the exact total and the bar graph
// after it is scaled to fit the terminal. The trailing "N files changed"
// summary line has no pipe and is skipped.
func parseStat(out string) map[string][2]int {
	counts := map[string][2]int{}
	for _, line := range strings.Split(out, "\n") {
		bar := strings.LastIndex(line, "|")
		if bar < 0 {
			continue
		}
		_, path := expandBraces(strings.TrimSpace(line[:bar]))
		fields := strings.Fields(line[bar+1:])
		if len(fields) == 0 {
			continue
		}
		total, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		var plus, minus int
		if len(fields) > 1 {
			plus = strings.Count(fields[1], "+")
			minus = strings.Count(fields[1], "-")
		}
		switch {
		case plus+minus == 0:
			counts[path] = [2]int{total, 0}
		case plus+minus == total:
			counts[path] = [2]int{plus, minus}
		default:
			// The graph was scaled down; recover the split from its ratio.
			added := (plus*total + (plus+minus)/2) / (plus + minus)
			counts[path] = [2]int{added, total - added}
		}
	}
	return counts
}

// expandBraces turns git's "dir/{a.txt => b.txt}" rename shorthand into the
// old and new paths. A path without braces is returned as the new path alone.
func expandBraces(s string) (oldPath, newPath string) {
	arrow := strings.Index(s, " => ")
	if arrow < 0 {
		return "", s
	}
	open := strings.Index(s, "{")
	if open < 0 || open > arrow {
		return strings.TrimSpace(s[:arrow]), strings.TrimSpace(s[arrow+4:])
	}
	shut := strings.Index(s[arrow:], "}")
	if shut < 0 {
		return "", s
	}
	shut += arrow
	prefix, suffix := s[:open], s[shut+1:]
	return prefix + s[open+1:arrow] + suffix, prefix + s[arrow+4:shut] + suffix
}

// DiffStream fetches the git-format diff for every file, as jj writes it.
func (r *Repo) DiffStream(ctx context.Context, spec vcs.DiffSpec, context int) (io.ReadCloser, error) {
	if spec.Empty() {
		return io.NopCloser(strings.NewReader("")), nil
	}
	return r.streamRead(ctx, append(args(spec), "--git", "--context", strconv.Itoa(context))...)
}

// FileDiff fetches the git-format diff for a single path.
func (r *Repo) FileDiff(ctx context.Context, spec vcs.DiffSpec, path string, context int) (string, error) {
	full := append(args(spec), "--git", "--context", strconv.Itoa(context), path)
	out, err := r.read(ctx, full...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FileContent returns one side of a file in full. jj spells the parent of a
// revision with a trailing dash.
func (r *Repo) FileContent(ctx context.Context, spec vcs.DiffSpec, path string, side vcs.Side) ([]byte, error) {
	before := side == vcs.Before
	rev := spec.Rev
	switch {
	case spec.Kind == vcs.DiffRange && before:
		rev = spec.From
	case spec.Kind == vcs.DiffRange:
		rev = spec.To
	case before:
		rev = spec.Rev + "-"
	}
	return r.read(ctx, "file", "show", "-r", rev, path)
}
