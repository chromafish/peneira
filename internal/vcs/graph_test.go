package vcs

import "testing"

// rev is a revision with only the fields BuildGraph reads.
func rev(id string, parents ...string) Revision {
	return Revision{ChangeID: id, Parents: parents}
}

func TestBuildGraphLinear(t *testing.T) {
	revs := []Revision{rev("c", "b"), rev("b", "a"), rev("a")}
	BuildGraph(revs)
	for i, r := range revs {
		if r.Graph.Column != 0 || r.Graph.Width != 1 {
			t.Errorf("revision %d drawn at column %d width %d, want a single lane", i, r.Graph.Column, r.Graph.Width)
		}
		if len(r.Graph.In) != 0 || len(r.Graph.Through) != 0 {
			t.Errorf("revision %d has crossings: %+v", i, r.Graph)
		}
	}
	if got := revs[2].Graph.Out; len(got) != 0 {
		t.Errorf("the root should have no outgoing lanes, got %v", got)
	}
}

// The shape here is the one jj itself draws for a merge of two branches that
// both descend from the same commit, with an unrelated head alongside.
func TestBuildGraphMerge(t *testing.T) {
	revs := []Revision{
		rev("merge", "featA", "featB"),
		rev("featB", "base"),
		rev("featA", "base"),
		rev("head", "base"),
		rev("base", "root"),
		rev("root"),
	}
	BuildGraph(revs)

	if got := revs[0].Graph.Out; len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("merge fans out to %v, want lanes 0 and 1", got)
	}
	if revs[1].Graph.Column != 1 {
		t.Errorf("featB drawn in lane %d, want 1", revs[1].Graph.Column)
	}
	if got := revs[1].Graph.Through; len(got) != 1 || got[0] != 0 {
		t.Errorf("featA's lane should pass through featB's row, got %v", got)
	}
	if revs[2].Graph.Column != 0 {
		t.Errorf("featA drawn in lane %d, want 0", revs[2].Graph.Column)
	}
	if revs[3].Graph.Column != 2 {
		t.Errorf("the unrelated head should open a new lane, got %d", revs[3].Graph.Column)
	}
	// Every lane waiting for base converges on it.
	if got := revs[4].Graph.In; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("base collects %v, want lanes 1 and 2", got)
	}
	if revs[4].Graph.Column != 0 {
		t.Errorf("base drawn in lane %d, want 0", revs[4].Graph.Column)
	}
}

// A revset can exclude a revision's parents, which leaves rows whose parents
// are never drawn. The graph must not keep those lanes open forever.
func TestBuildGraphMissingParents(t *testing.T) {
	revs := []Revision{rev("x", "missing"), rev("y", "alsomissing")}
	BuildGraph(revs)
	if revs[0].Graph.Column != 0 {
		t.Errorf("first row at lane %d, want 0", revs[0].Graph.Column)
	}
	if revs[1].Graph.Column != 1 {
		t.Errorf("second row should open its own lane, got %d", revs[1].Graph.Column)
	}
	if got := revs[1].Graph.Through; len(got) != 1 || got[0] != 0 {
		t.Errorf("the first row's dangling lane should pass through, got %v", got)
	}
}
