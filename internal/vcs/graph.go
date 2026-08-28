package vcs

// GraphRow describes how one revision is drawn in the log's ancestry graph.
//
// A row is treated as two halves. Lines in In arrive at the top edge and stop
// at the node in the middle; lines in Out leave the node and continue past the
// bottom edge; lines in Through belong to unrelated branches and cross the row
// untouched. That split is what lets a merge fan in and a fork fan out within
// a single row of fixed height.
type GraphRow struct {
	Column  int
	In      []int
	Through []int
	Out     []int
	Width   int // number of lanes occupied anywhere in this row
}

// BuildGraph assigns lanes to a topologically ordered, newest-first list of
// revisions and fills in each one's GraphRow. It works purely from the parent
// change IDs already carried by each revision, so the graph stays correct for
// any revset rather than depending on jj's own ASCII rendering.
//
// Revisions are modified in place.
func BuildGraph(revs []Revision) {
	// lanes[c] is the change ID that lane c is currently waiting to reach,
	// i.e. a parent that some revision already drawn above still needs.
	// An empty string means the lane is free.
	var lanes []string

	free := func() int {
		for c, id := range lanes {
			if id == "" {
				return c
			}
		}
		lanes = append(lanes, "")
		return len(lanes) - 1
	}

	for i := range revs {
		rev := &revs[i]

		// The node sits in whichever lane was already waiting for it. A
		// revision nothing below has claimed is a new head and opens a lane.
		col := -1
		for c, id := range lanes {
			if id == rev.ChangeID {
				col = c
				break
			}
		}
		if col == -1 {
			col = free()
		}

		var row GraphRow
		row.Column = col
		for c, id := range lanes {
			switch {
			case id == rev.ChangeID:
				// Every lane waiting for this change converges here. The
				// node's own lane is the trivial case and needs no line.
				if c != col {
					row.In = append(row.In, c)
				}
				lanes[c] = ""
			case id != "":
				row.Through = append(row.Through, c)
			}
		}

		// The first parent inherits the node's lane so mainline history stays
		// in a straight column; further parents branch off into free lanes.
		for n, parent := range rev.Parents {
			target := col
			if n > 0 {
				target = free()
			}
			lanes[target] = parent
			row.Out = append(row.Out, target)
		}

		// Trailing free lanes are not drawn, so the row is only as wide as its
		// rightmost occupied lane.
		row.Width = len(lanes)
		for row.Width > 0 && lanes[row.Width-1] == "" {
			row.Width--
		}
		if row.Width <= col {
			row.Width = col + 1
		}
		for _, c := range row.In {
			if c >= row.Width {
				row.Width = c + 1
			}
		}
		rev.Graph = row
	}
}
