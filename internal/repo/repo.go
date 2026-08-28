// Package repo opens whichever kind of repository is at a path.
//
// It is separate from internal/vcs so that a backend can depend on the types
// without depending on the other backend.
package repo

import (
	"context"
	"fmt"

	"github.com/chromafish/peneira/internal/git"
	"github.com/chromafish/peneira/internal/jj"
	"github.com/chromafish/peneira/internal/vcs"
)

// Open finds the repository containing dir and opens it with the backend that
// owns it.
//
// jj is tried first, so a colocated repository — a jj repository with a git
// one inside it — is opened as a jj repository. jj is what rewrote those
// commits, and it is the only one of the two that can say so.
func Open(ctx context.Context, dir string) (vcs.Repo, error) {
	if r, err := jj.Open(ctx, dir); err == nil {
		return r, nil
	}
	if r, err := git.Open(ctx, dir); err == nil {
		return r, nil
	}
	return nil, fmt.Errorf("%s is not inside a jj or git repository", dir)
}
