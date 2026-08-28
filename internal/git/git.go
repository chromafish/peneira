// Package git reads a git repository through the git command line.
//
// It is the second of the two backends behind internal/vcs. Where jj can say
// that a change was rewritten, that a commit is immutable or that the working
// copy is a commit like any other, git cannot, and this package leaves those
// marks unset rather than inventing them. The one place it does synthesise
// something is the working tree, which appears as a row at the head of the log
// whenever there is uncommitted work — see Log.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromafish/peneira/internal/proc"
	"github.com/chromafish/peneira/internal/vcs"
)

// Field and record separators for log formats. They cannot occur in commit
// metadata, so nothing has to be escaped.
const (
	fs = "\x1f"
	rs = "\x1e"
)

// workingTree is the revision identifier of the synthetic row standing for
// uncommitted work. No commit can collide with it: a git object name is hex.
const workingTree = "working"

// Repo is a handle on a git repository.
type Repo struct {
	root    string
	bin     string // resolved git executable
	version string
}

// Open opens the git repository containing dir.
func Open(ctx context.Context, dir string) (*Repo, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not found in PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, bin, "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git repository: %w", dir, proc.Clean(err))
	}
	r := &Repo{root: strings.TrimSpace(string(out)), bin: bin}
	r.version = r.readVersion(ctx)
	return r, nil
}

// Root is the absolute path of the repository's top directory.
func (r *Repo) Root() string { return r.root }

// Info describes the backend to the interface. git takes the arguments git log
// takes, and an agent is told to check a commit out.
func (r *Repo) Info() vcs.Info {
	return vcs.Info{
		Name:       "git",
		Version:    r.version,
		QueryLabel: "REVS",
		QueryHint:  "(HEAD and its ancestors)",
		Handoff: "The commit is already checked out if `git rev-parse HEAD` starts with %[1]s;\n" +
			"otherwise run `git checkout %[1]s` before making the edits below.",
	}
}

// readVersion reports the git version, e.g. "2.43.0", or "" if git will not
// say.
func (r *Repo) readVersion(ctx context.Context) string {
	out, err := r.read(ctx, "--version")
	if err != nil {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(string(out)), "git version "))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// Snapshot does nothing. git has no working copy to record: what is on disk is
// what the next command will read.
func (r *Repo) Snapshot(context.Context) error { return nil }

// read runs git and returns its output.
func (r *Repo) read(ctx context.Context, args ...string) ([]byte, error) {
	return proc.Run(ctx, r.root, r.bin, args...)
}

// The log format, one field per line of the template, in the order parseLog
// reads them. Refs are asked for in full so that a local branch called
// feature/x cannot be mistaken for a remote one.
const logFormat = "%H" + fs + "%h" + fs + "%an" + fs + "%ae" + fs + "%aI" + fs +
	"%p" + fs + "%D" + fs + "%s" + rs

// Log lists revisions, newest first. The query is passed to git log as it was
// typed, so anything git log takes works: a range, a branch name, --author, a
// path after --. An empty query is git's own default, HEAD and its ancestors.
//
// When the working tree has uncommitted changes and HEAD is among the
// revisions returned, a row for that work is put at the head of the list. It
// is not a commit and has no commit ID, but it is what you are looking at
// before you commit, and it is the row a diff of the working tree hangs from.
func (r *Repo) Log(ctx context.Context, query string, limit int) ([]vcs.Revision, error) {
	args := []string{"log", "--topo-order", "--decorate=full", "--format=" + logFormat}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	args = append(args, strings.Fields(query)...)

	out, err := r.read(ctx, args...)
	if err != nil {
		return nil, err
	}
	revs, err := parseLog(string(out))
	if err != nil {
		return nil, err
	}
	if w, ok := r.workingRow(ctx, revs); ok {
		revs = append([]vcs.Revision{w}, revs...)
	}
	return revs, nil
}

// workingRow builds the row standing for uncommitted work, if there is any and
// if the commit it sits on is on screen. A dirty tree under a query that does
// not include HEAD is not this list's business.
func (r *Repo) workingRow(ctx context.Context, revs []vcs.Revision) (vcs.Revision, bool) {
	head, err := r.read(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		return vcs.Revision{}, false
	}
	parent := strings.TrimSpace(string(head))

	var onScreen bool
	for _, rev := range revs {
		if rev.ChangeID == parent {
			onScreen = true
			break
		}
	}
	if !onScreen {
		return vcs.Revision{}, false
	}

	status, err := r.read(ctx, "status", "--porcelain")
	if err != nil || len(bytes.TrimSpace(status)) == 0 {
		return vcs.Revision{}, false
	}
	return vcs.Revision{
		ChangeID:     workingTree,
		ChangeIDFull: workingTree,
		Description:  "(uncommitted changes)",
		Timestamp:    time.Now(),
		WorkingCopy:  true,
		Conflict:     unmerged(string(status)),
		Parents:      []string{parent},
	}, true
}

// unmerged reports whether a porcelain status holds a conflicted path, which
// is the one sense in which a git working tree can be said to be in conflict.
func unmerged(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			return true
		}
	}
	return false
}

func parseLog(out string) ([]vcs.Revision, error) {
	var revs []vcs.Revision
	for _, rec := range strings.Split(out, rs) {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, fs)
		if len(f) < 8 {
			return nil, fmt.Errorf("malformed log record with %d fields", len(f))
		}
		rev := vcs.Revision{
			ChangeID:     f[1],
			ChangeIDFull: f[0],
			CommitID:     f[1],
			CommitIDFull: f[0],
			Author:       f[2],
			Email:        f[3],
			Description:  f[7],
			Parents:      strings.Fields(f[5]),
		}
		if t, err := time.Parse(time.RFC3339, f[4]); err == nil {
			rev.Timestamp = t
		}
		rev.Root = len(rev.Parents) == 0
		rev.Bookmarks, rev.RemoteBookmarks, rev.Tags = parseRefs(f[6])
		revs = append(revs, rev)
	}
	return revs, nil
}

// parseRefs reads the full ref names git decorates a commit with, e.g.
// "HEAD -> refs/heads/main, refs/remotes/origin/main, refs/tags/v1".
func parseRefs(s string) (local, remote, tags []string) {
	for _, ref := range strings.Split(s, ",") {
		ref = strings.TrimSpace(ref)
		if _, after, found := strings.Cut(ref, " -> "); found {
			ref = after
		}
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			local = append(local, strings.TrimPrefix(ref, "refs/heads/"))
		case strings.HasPrefix(ref, "refs/remotes/"):
			remote = append(remote, strings.TrimPrefix(ref, "refs/remotes/"))
		case strings.HasPrefix(ref, "refs/tags/"):
			tags = append(tags, strings.TrimPrefix(ref, "refs/tags/"))
		}
	}
	return local, remote, tags
}

// Description returns the full commit message of a revision.
func (r *Repo) Description(ctx context.Context, rev string) (string, error) {
	if rev == workingTree {
		return "", nil
	}
	out, err := r.read(ctx, "log", "-1", "--format=%B", rev)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// path is a repository path as an absolute one, for reading the working tree.
func (r *Repo) path(p string) string { return filepath.Join(r.root, p) }
