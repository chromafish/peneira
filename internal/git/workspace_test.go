package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceIsMovedBetweenRevisions(t *testing.T) {
	dir := newRepo(t)
	r := open(t, dir)
	ctx := context.Background()
	revs, err := r.Log(ctx, "", 10)
	if err != nil || len(revs) != 2 {
		t.Fatalf("log: %v, %d revisions", err, len(revs))
	}
	second, first := revs[0].ChangeID, revs[1].ChangeID

	ws := filepath.Join(t.TempDir(), "cache", "repo-1234")
	if err := r.Workspace(ctx, ws, first); err != nil {
		t.Fatalf("adding the workspace: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(ws, "main.go"))
	if err != nil || !strings.Contains(string(body), `"one"`) {
		t.Fatalf("at the first commit main.go = %q, %v", body, err)
	}

	// A build leaves something behind, and something it overwrote.
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte("clobbered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "artefact"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Workspace(ctx, ws, second); err != nil {
		t.Fatalf("moving the workspace: %v", err)
	}
	body, _ = os.ReadFile(filepath.Join(ws, "main.go"))
	if !strings.Contains(string(body), `"two"`) {
		t.Errorf("at the second commit main.go = %q", body)
	}
	if _, err := os.Stat(filepath.Join(ws, "artefact")); err != nil {
		t.Error("moving the workspace threw away what the build left in it")
	}

	// The repository's own tree is not disturbed.
	body, _ = os.ReadFile(filepath.Join(dir, "main.go"))
	if !strings.Contains(string(body), `"two"`) {
		t.Errorf("the main working tree was changed: %q", body)
	}

	// A directory that went away is added again rather than refused.
	if err := os.RemoveAll(ws); err != nil {
		t.Fatal(err)
	}
	if err := r.Workspace(ctx, ws, first); err != nil {
		t.Fatalf("re-adding the workspace: %v", err)
	}
	body, _ = os.ReadFile(filepath.Join(ws, "main.go"))
	if !strings.Contains(string(body), `"one"`) {
		t.Errorf("after re-adding main.go = %q", body)
	}
}
