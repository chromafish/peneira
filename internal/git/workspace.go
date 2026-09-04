package git

import (
	"context"
	"os"
	"path/filepath"

	"github.com/chromafish/peneira/internal/proc"
)

// Workspace adds dir as a linked worktree the first time, detached at rev,
// and checks rev out in it after that. The checkout is forced: whatever a
// build overwrote in the worktree is the worktree's to lose.
func (r *Repo) Workspace(ctx context.Context, dir, rev string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		_, err := proc.Run(ctx, dir, r.bin, "checkout", "--force", "--detach", rev)
		return err
	}
	// A worktree whose directory has gone is still on the books, and blocks
	// adding one at the same path.
	r.read(ctx, "worktree", "prune")
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	_, err := r.read(ctx, "worktree", "add", "--detach", dir, rev)
	return err
}
