package jj

import (
	"context"
	"os"
	"path/filepath"

	"github.com/chromafish/check/internal/proc"
)

// Workspace adds dir as a jj workspace of the repository the first time, and
// moves its working copy to rev after that. The workspace takes the name of
// its directory.
//
// The workspace sits on a working-copy commit of its own on top of rev, so
// rev is never rewritten by a build. jj tracks every file that is not
// ignored, so anything a build leaves behind that the repository does not
// ignore lands in that commit; it is abandoned before the workspace moves on,
// rather than left as a stray head in the log. Ignored output stays on disk.
func (r *Repo) Workspace(ctx context.Context, dir, rev string) error {
	if _, err := os.Stat(filepath.Join(dir, ".jj")); err == nil {
		return r.move(ctx, dir, rev)
	}
	// A workspace whose directory has gone is still on the books under its
	// name, and blocks adding it again.
	r.run(ctx, "workspace", "forget", filepath.Base(dir))
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	_, err := r.run(ctx, "workspace", "add", "--name", filepath.Base(dir), "-r", rev, dir)
	return err
}

func (r *Repo) move(ctx context.Context, dir, rev string) error {
	in := func(args ...string) error {
		_, err := proc.Run(ctx, dir, r.bin, append([]string{"--no-pager", "--color=never"}, args...)...)
		return err
	}
	move := func() error {
		if err := in("abandon", "@"); err != nil {
			return err
		}
		return in("new", rev)
	}
	err := move()
	if err == nil {
		return nil
	}
	// An operation in the main workspace can leave this one's working copy
	// stale, and jj refuses to go on until it is brought up to date.
	if in("workspace", "update-stale") != nil {
		return err
	}
	return move()
}
