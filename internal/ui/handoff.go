package ui

import (
	"context"
	"fmt"
	"image"
	"io"
	"strings"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/chromafish/peneira/internal/notes"
	"github.com/chromafish/peneira/internal/state"

	"github.com/chromafish/peneira/internal/vcs"
	"github.com/chromafish/peneira/reef"
)

// fileSource lets the exporter read either side of a file out of the
// repository at the revision under review.
type fileSource struct {
	repo vcs.Repo
	spec vcs.DiffSpec
}

func (f fileSource) FileLines(ctx context.Context, path string, old bool) ([]string, error) {
	side := vcs.After
	if old {
		side = vcs.Before
	}
	raw, err := f.repo.FileContent(ctx, f.spec, path, side)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n"), nil
}

// openCount is how many notes are waiting, for the chrome that only needs the
// number.
func (a *App) openCount() int {
	if a.store == nil || a.rev.ChangeIDFull == "" {
		return 0
	}
	return a.store.OpenCount(a.rev.ChangeIDFull)
}

// openNotes returns the unresolved notes on the change under review. Resolved
// ones are deliberately left out: they are the ones already dealt with.
func (a *App) openNotes() []state.Comment {
	if a.store == nil || a.rev.ChangeIDFull == "" {
		return nil
	}
	var out []state.Comment
	for _, c := range a.store.AllComments(a.rev.ChangeIDFull) {
		if !c.Resolved {
			out = append(out, c)
		}
	}
	return out
}

// copyNotes renders every unresolved note, with the code it refers to, and puts
// the result on the clipboard ready to paste into an agent.
//
// Reading the snippets means going back to the repository, so the work happens
// off the main goroutine and the clipboard write is left for the next frame.
func (a *App) copyNotes() {
	open := a.openNotes()
	if len(open) == 0 {
		a.note("no notes to copy")
		return
	}
	change := notes.Change{
		Handoff:     a.backend.Handoff,
		RepoRoot:    a.repo.Root(),
		ChangeID:    a.rev.ChangeID,
		CommitID:    a.rev.CommitID,
		Description: a.desc,
	}

	src := fileSource{repo: a.repo, spec: a.spec}

	out := make([]notes.Note, len(open))
	for i, c := range open {
		out[i] = notes.Note{
			Path:    c.Path,
			Old:     c.Side == state.SideOld,
			Line:    c.Line,
			EndLine: c.EndLine,
			Body:    c.Body,
		}
	}

	a.background(func(ctx context.Context) func() {
		text := notes.Render(ctx, change, out, src)
		return func() {
			a.clipboard = text
			a.note("copied %d notes — paste into your agent", len(out))
		}
	})
}

// clearNotes throws away every note on the change. There is no confirmation,
// as with deleting a single note; the status bar reports the count.
func (a *App) clearNotes() {
	n := a.store.ClearComments(a.rev.ChangeIDFull)
	if n == 0 {
		a.note("no notes to clear")
		return
	}
	a.notesOpen = false
	a.afterCommentChange("")
	if n == 1 {
		a.note("1 note cleared")
		return
	}
	a.note("%d notes cleared", n)
}

// flushClipboard writes any pending text. Clipboard writes are commands issued
// during a frame, so they cannot be made from a background goroutine.
func (a *App) flushClipboard(gtx layout.Context) {
	if a.clipboard == "" {
		return
	}
	gtx.Execute(clipboard.WriteCmd{
		Type: "application/text",
		Data: io.NopCloser(strings.NewReader(a.clipboard)),
	})
	a.clipboard = ""
}

// layoutNotes draws the sheet listing every note on the change, with the one
// button that matters on it.
func (a *App) layoutNotes(gtx layout.Context) {
	if !a.notesOpen {
		return
	}
	ui := a.ui
	size := gtx.Constraints.Max
	row := ui.TextRow(gtx, reef.SizeUI)
	cell := ui.Cell(gtx, reef.SizeUI, false)
	pad := gtx.Dp(reef.PadCard)

	open := a.openNotes()
	cols := max(20, (min(size.X-gtx.Dp(120), 100*cell.X)-pad*2)/cell.X)

	// Each note takes a line for its location and however many its body wraps
	// to, so the sheet is only as tall as it needs to be.
	bodies := make([][]string, len(open))
	lines := 0
	for i, n := range open {
		bodies[i] = reef.Wrap(n.Body, cols-2)
		lines += 1 + len(bodies[i])
	}

	w := min(size.X-gtx.Dp(120), 100*cell.X)
	h := min(size.Y-gtx.Dp(100), row*(lines+4)+pad*2)
	x, y := (size.X-w)/2, (size.Y-h)/2

	sheet := image.Rect(x, y, x+w, y+h)
	ui.Sheet(gtx, sheet)

	ly := y + pad
	off := op.Offset(image.Pt(0, ly)).Push(gtx.Ops)
	ui.LabelAt(gtx, ui.P.Strong, x+pad, row, x+w-pad, fmt.Sprintf("NOTES · %d OPEN", len(open)))
	off.Pop()

	// Copying and clearing sit together on the header line: handing the notes
	// over and being done with them is one motion.
	if len(open) > 0 {
		fill(gtx, image.Pt(0, ly), image.Pt(x+w-pad, row), func(gtx layout.Context) {
			rightX := x + w - pad
			rightX -= a.controlRight(gtx, rightX, row, &a.notesOpen, "COPY FOR AGENT  ⌘⇧C", ui.P.Ok, func() {
				a.copyNotes()
				a.notesOpen = false
			}) + gtx.Dp(reef.Sp3)
			a.controlRight(gtx, rightX, row, tagClearNotes, "CLEAR ALL", ui.P.Error, a.clearNotes)
		})
	}
	ly += row + row/2

	if len(open) == 0 {
		a.sheetText(gtx, x+pad, ly, x+w-pad, row, font.Normal, ui.P.Faint,
			"Nothing yet. Press c on a line in the diff.")
		return
	}

	for i, n := range open {
		if ly+row*(1+len(bodies[i])) > y+h-pad {
			a.sheetText(gtx, x+pad, ly, x+w-pad, row, font.Normal, ui.P.Faint,
				fmt.Sprintf("… and %d more", len(open)-i))
			break
		}
		side := ""
		if n.Side == state.SideOld {
			side = "  (before)"
		}
		a.sheetText(gtx, x+pad, ly, x+w-pad, row, reef.WeightLabel, ui.P.Fg,
			fmt.Sprintf("%s:%d%s", n.Path, n.Line, side))
		ly += row
		for _, line := range bodies[i] {
			a.sheetText(gtx, x+pad+cell.X*2, ly, x+w-pad, row, font.Normal, ui.P.Muted, line)
			ly += row
		}
	}
}

// sheetText draws one line inside an overlay sheet, on the band that starts
// at y.
func (a *App) sheetText(gtx layout.Context, x, y, limit, row int, weight font.Weight, c reef.ColorNRGBA, txt string) {
	off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	a.cellText(gtx, x, row, limit, weight, c, txt)
	off.Pop()
}
