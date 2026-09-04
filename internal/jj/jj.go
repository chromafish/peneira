// Package jj reads a jujutsu repository through the jj command line.
//
// Every query goes through run, which pins the working directory to the repo
// root and disables colour and paging so the output is stable to parse. Reads
// default to --ignore-working-copy: jj would otherwise snapshot the working
// copy on every single query, which turns browsing a repository into a stream
// of operations in the op log. Snapshot makes that explicit instead.
package jj

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/chromafish/peneira/internal/proc"
	"github.com/chromafish/peneira/internal/vcs"
)

// Field and record separators used in log templates. jj passes them through
// verbatim, and they cannot occur in commit metadata.
const (
	fs = "\x1f"
	rs = "\x1e"
)

// Repo is a handle on a jujutsu repository.
type Repo struct {
	root    string
	bin     string // resolved jj executable
	version string
}

// Open locates the jj repository containing dir.
func Open(ctx context.Context, dir string) (*Repo, error) {
	bin, err := exec.LookPath("jj")
	if err != nil {
		return nil, fmt.Errorf("jj not found in PATH: %w", err)
	}
	out, err := proc.Run(ctx, dir, bin, "root")
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a jj repository: %w", dir, err)
	}
	r := &Repo{root: strings.TrimSpace(string(out)), bin: bin}
	r.version = r.readVersion(ctx)
	return r, nil
}

// readVersion reports the jj version, e.g. "0.44.0", or "" if jj will not say.
func (r *Repo) readVersion(ctx context.Context) string {
	out, err := r.run(ctx, "--version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(string(out), "jj "))
}

// run executes jj under the global flags every query needs: no pager, no
// colour.
func (r *Repo) run(ctx context.Context, args ...string) ([]byte, error) {
	return proc.Run(ctx, r.root, r.bin, append([]string{"--no-pager", "--color=never"}, args...)...)
}

// read is run with --ignore-working-copy, for queries that must not mutate the
// repository.
func (r *Repo) read(ctx context.Context, args ...string) ([]byte, error) {
	return r.run(ctx, append([]string{"--ignore-working-copy"}, args...)...)
}

// Snapshot updates the working-copy commit from the files on disk. This is the
// one operation the app performs that writes to the repo, and it only ever runs
// on an explicit refresh.
func (r *Repo) Snapshot(ctx context.Context) error {
	_, err := r.run(ctx, "status")
	return err
}

// streamRead starts a read that is returned as it is produced, under the same
// global flags run uses.
func (r *Repo) streamRead(ctx context.Context, args ...string) (io.ReadCloser, error) {
	full := append([]string{"--no-pager", "--color=never", "--ignore-working-copy"}, args...)
	return proc.Stream(ctx, r.root, r.bin, full...)
}

// Root is the absolute path of the repository's top directory.
func (r *Repo) Root() string { return r.root }

// Info describes the backend to the interface. jj takes a revset, and an agent
// is told to move to a change with jj edit.
func (r *Repo) Info() vcs.Info {
	return vcs.Info{
		Name:       "jj",
		Version:    r.version,
		QueryLabel: "REVSET",
		QueryHint:  "(all revisions)",
		// jj's own default is the working copy and a couple of generations
		// around it, which leaves a tall pane nearly empty. root() is jj's
		// synthetic commit below every history and has nothing to show.
		DefaultQuery: "all() ~ root()",
		Handoff: "The change is already the working copy if `jj log -r %[1]s` shows it as `@`;\n" +
			"otherwise run `jj edit %[1]s` before making the edits below.",
	}
}

const logTemplate = `change_id.short(8) ++ "` + fs + `" ++ change_id ++ "` + fs + `" ++
	commit_id.short(8) ++ "` + fs + `" ++ commit_id ++ "` + fs + `" ++
	description.first_line() ++ "` + fs + `" ++
	author.name() ++ "` + fs + `" ++ author.email() ++ "` + fs + `" ++
	author.timestamp().format("%Y-%m-%dT%H:%M:%S%:z") ++ "` + fs + `" ++
	if(current_working_copy,"1","0") ++ if(immutable,"1","0") ++ if(conflict,"1","0") ++
	if(empty,"1","0") ++ if(root,"1","0") ++ if(divergent,"1","0") ++ if(hidden,"1","0") ++ "` + fs + `" ++
	local_bookmarks.map(|b| b.name()).join(",") ++ "` + fs + `" ++
	remote_bookmarks.map(|b| b.name() ++ "@" ++ b.remote()).join(",") ++ "` + fs + `" ++
	tags.map(|t| t.name()).join(",") ++ "` + fs + `" ++
	parents.map(|c| c.change_id().short(8)).join(",") ++ "` + rs + `"`

// Log evaluates a revset and returns the matching revisions, newest first.
// An empty revset uses jj's own default (the visible heads and their ancestry).
func (r *Repo) Log(ctx context.Context, revset string, limit int) ([]vcs.Revision, error) {
	args := []string{"log", "--no-graph", "-T", logTemplate}
	if revset != "" {
		args = append(args, "-r", revset)
	}
	if limit > 0 {
		args = append(args, "--limit", fmt.Sprint(limit))
	}
	out, err := r.read(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseLog(string(out))
}

func parseLog(out string) ([]vcs.Revision, error) {
	var revs []vcs.Revision
	for _, rec := range strings.Split(out, rs) {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, fs)
		if len(f) < 13 {
			return nil, fmt.Errorf("malformed log record with %d fields", len(f))
		}
		rev := vcs.Revision{
			ChangeID:     f[0],
			ChangeIDFull: f[1],
			CommitID:     f[2],
			CommitIDFull: f[3],
			Description:  f[4],
			Author:       f[5],
			Email:        f[6],
		}
		if t, err := time.Parse("2006-01-02T15:04:05-07:00", f[7]); err == nil {
			rev.Timestamp = t
		}
		flags := f[8]
		at := func(i int) bool { return i < len(flags) && flags[i] == '1' }
		rev.WorkingCopy, rev.Immutable, rev.Conflict = at(0), at(1), at(2)
		rev.Empty, rev.Root, rev.Divergent, rev.Hidden = at(3), at(4), at(5), at(6)
		rev.Bookmarks = splitList(f[9])
		rev.RemoteBookmarks = splitList(f[10])
		rev.Tags = splitList(f[11])
		rev.Parents = splitList(f[12])
		revs = append(revs, rev)
	}
	return revs, nil
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// Description returns the full, multi-line description of a revision.
func (r *Repo) Description(ctx context.Context, rev string) (string, error) {
	out, err := r.read(ctx, "log", "--no-graph", "-r", rev, "-T", "description")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}
