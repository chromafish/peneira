package ui

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/chromafish/check/internal/repo"
	"github.com/chromafish/check/internal/state"
	"github.com/chromafish/check/internal/vcs"

	"github.com/chromafish/check/reef"
)

// layoutOpen draws the screen shown when no repository is loaded: a way to
// choose one, and the ones opened before.
func (a *App) layoutOpen(gtx layout.Context) {
	ui := a.ui
	size := gtx.Constraints.Max
	row := ui.Row(gtx)
	pad := gtx.Dp(reef.Gutter)
	cell := ui.Cell(gtx, reef.SizeUI, false)

	w := min(size.X-gtx.Dp(80), 72*cell.X)
	rows := len(a.recent) + 4
	h := min(size.Y-gtx.Dp(80), row*rows+pad*2)
	x, y := (size.X-w)/2, max(gtx.Dp(24), (size.Y-h)/3)

	// The sheet sits on a dot-grid desk, which is what makes it read as a
	// sheet: the paper edge is visible, and the drop under it is a printed
	// offset rather than a glow.
	ui.DotGrid(gtx, size)
	sheet := image.Rect(x, y, x+w, y+h)
	ui.Sheet(gtx, sheet)

	// One heading of the sheet, on the band that starts at ly.
	label := func(ly int, c reef.ColorNRGBA, txt string) {
		off := op.Offset(image.Pt(0, ly)).Push(gtx.Ops)
		ui.LabelAt(gtx, c, x+pad, row, x+w-pad, txt)
		off.Pop()
	}
	text := func(ly, indent int, weight font.Weight, c reef.ColorNRGBA, txt string) {
		fit(gtx, image.Pt(0, ly), image.Pt(x+w-pad, row), func(gtx layout.Context) {
			a.cellText(gtx, x+pad+indent, row, x+w-pad, weight, c, txt)
		})
	}

	ly := y + pad
	label(ly, ui.P.Strong, "OPEN A REPOSITORY")
	ly += row

	if a.openErr != "" {
		label(ly, ui.P.Error, "ERROR "+firstLine(a.openErr))
	} else {
		text(ly, 0, font.Normal, ui.P.Muted, "⌘O  choose a folder…")
	}
	ly += row

	if len(a.recent) == 0 {
		ly += row
		text(ly, 0, font.Normal, ui.P.Faint, "Nothing opened yet.")
		return
	}

	ly += row / 2
	label(ly, ui.P.Faint, "RECENT")
	ly += row

	for i, r := range a.recent {
		if ly+row > y+h-pad/2 {
			break
		}
		rowRect := image.Rect(x+gtx.Dp(reef.Sp3), ly, x+w-gtx.Dp(reef.Sp3), ly+row)
		if i == a.recentSel {
			ui.SelectionRect(gtx, rowRect, true)
		}
		// Each entry is clickable as well as reachable with the arrow keys.
		fill(gtx, rowRect.Min, rowRect.Size(), func(gtx layout.Context) {
			a.clickable(gtx, rowRect.Size(), &a.recent[i], func() {
				a.recentSel = i
				a.openRepo(r.Path)
			})
		})

		fg, muted := ui.P.Fg, ui.P.Faint
		text(ly, 0, reef.WeightLabel, fg, r.Name())
		text(ly, (len(r.Name())+2)*cell.X, font.Normal, muted, shortenHome(filepath.Dir(r.Path)))
		ly += row
	}
}

// shortenHome writes a path under the home directory with a tilde, the way it
// would be typed.
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, found := strings.CutPrefix(path, home+string(filepath.Separator)); found {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// chooseFolder runs the system directory chooser and opens what comes back.
// Only one chooser is ever in flight: a second request while a panel is up
// would sit behind it and then open a panel nobody asked for any more, which
// is worse than the click appearing to do nothing.
func (a *App) chooseFolder() {
	if a.picking {
		return
	}
	start := ""
	if a.repo != nil {
		start = a.repo.Root()
	} else if len(a.recent) > 0 {
		start = a.recent[0].Path
	}
	a.picking = true
	a.background(func(ctx context.Context) func() {
		path, err := pickFolder(ctx, start)
		return func() {
			a.picking = false
			if errors.Is(err, errPickCancelled) {
				return
			}
			if err != nil {
				a.openErr = err.Error()
				a.fail(err)
				return
			}
			a.openRepo(path)
		}
	})
}

// openRepo switches to another repository, keeping the window and the theme.
func (a *App) openRepo(path string) {
	a.background(func(ctx context.Context) func() {
		opened, err := repo.Open(ctx, path)
		if err != nil {
			return func() {
				a.openErr = err.Error()
				a.failure = err.Error()
			}
		}
		return func() {
			a.supersede()
			a.resetSonda()
			a.repo = opened
			a.dir = absDir(path, opened)
			a.adoptBackend(opened)
			a.store = state.New()
			a.repoName = filepath.Base(opened.Root())
			a.openErr = ""
			a.failure = ""
			a.recent = state.RememberRecent(opened.Root())
			a.recentSel = 0
			a.revs = nil
			a.files = nil
			a.diff = nil
			a.rev = vcs.Revision{}
			a.desc = ""
			a.focus = PaneRevs
			a.setTitle()
			a.reload(true)
			a.note("opened %s", opened.Root())
		}
	})
}

// moveRecent steps through the recent list on the open screen.
func (a *App) moveRecent(delta int) {
	if len(a.recent) == 0 {
		return
	}
	a.recentSel = clamp(a.recentSel+delta, 0, len(a.recent)-1)
}

func (a *App) openSelectedRecent() {
	if a.recentSel < len(a.recent) {
		a.openRepo(a.recent[a.recentSel].Path)
	}
}
