package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/chromafish/peneira/internal/sonda"
	"github.com/chromafish/peneira/internal/vcs"

	"github.com/chromafish/peneira/reef"
)

// Sonda is a screen of its own, entered from the review and left back to it.
// It holds two runs of one target: the baseline on the left, built from a
// second working copy at the revision the change is measured against, and
// the working copy under review on the right. What each run wrote is shown
// as it arrives, and what differs between them is what the review reads.
//
// The runs outlive the screen. Leaving it goes back to the diff with the
// programs still up; they stop when asked, or when the window closes.

type sondaScreen struct {
	open    bool
	targets []sonda.Target
	target  int

	filter   *reef.Field
	minLevel sonda.Severity
	raw      bool

	panes [2]logPane
	focus int

	// preparing is set while the baseline working copy is being moved, which
	// is the one step that has to finish before the baseline can be started.
	preparing bool
}

const (
	paneBaseline = 0
	paneReview   = 1
)

// logPane is one run as it is shown: the copy of its logs, the rows the
// filter lets through, and where the reading position is.
type logPane struct {
	label string
	rev   string
	note  string // why there is no run, when there is none

	run   *sonda.Run
	state sonda.State
	logs  []sonda.Log

	// view indexes the logs the filter lets through; seen is how many logs
	// have been judged against it, so a sync only judges what is new.
	view  []int
	seen  int
	built logFilter

	list   layout.List
	cursor int

	// follow keeps the cursor on the newest line as lines arrive. It is set
	// until the cursor is moved off the tail, and set again by moving it back.
	follow   bool
	overflow bool

	// Horizontal scroll, and the widest line, in cells.
	x, cols int
}

// logFilter is what both panes are seen through, so what is on screen stays
// comparable.
type logFilter struct {
	min  sonda.Severity
	text string
}

// pass reports whether a log is shown. The level applies only to logs that
// carry one: a line with no level is not guessed at, and a panic written as
// plain text has to stay in view under any level.
func (f logFilter) pass(l sonda.Log) bool {
	if l.Severity != sonda.Unlevelled && l.Severity < f.min {
		return false
	}
	return f.text == "" || strings.Contains(strings.ToLower(l.Raw), f.text)
}

func newSondaScreen() *sondaScreen {
	s := &sondaScreen{filter: reef.NewField("")}
	s.filter.Placeholder = "(substring)"
	s.panes[paneBaseline].reset("BASELINE", "")
	s.panes[paneReview].reset("REVIEW", "")
	for i := range s.panes {
		s.panes[i].list.Axis = layout.Vertical
	}
	return s
}

func (a *App) sondaOpen() bool { return a.sonda != nil && a.sonda.open }

// toggleSonda enters the screen, or leaves it back to the review.
func (a *App) toggleSonda() {
	if a.sondaOpen() {
		a.sonda.open = false
		a.ui.Release()
		return
	}
	a.openSonda()
}

// openSonda reads the targets afresh each time: the file is edited while
// the window is up, and a stale reading would run the wrong command.
func (a *App) openSonda() {
	if a.repo == nil {
		return
	}
	if a.sonda == nil {
		a.sonda = newSondaScreen()
	}
	s := a.sonda
	s.open = true
	a.help, a.notesOpen, a.settingsOpen = false, false, false
	a.ui.Release()

	targets, err := sonda.Load(a.dir)
	if err != nil {
		a.fail(err)
	}
	s.targets = targets
	if s.target >= len(targets) {
		s.target = 0
	}
	if !s.started() {
		s.target = a.pickTarget()
	}
}

func (s *sondaScreen) started() bool {
	return s.panes[paneBaseline].run != nil || s.panes[paneReview].run != nil
}

// pickTarget chooses the target the change reaches: the one whose directory
// holds a changed file, the deepest such where several do, and the first
// declared where none does.
func (a *App) pickTarget() int {
	best, depth := 0, -1
	for i, t := range a.sonda.targets {
		rel, err := filepath.Rel(a.repo.Root(), t.Dir)
		if err != nil {
			continue
		}
		if rel == "." {
			rel = ""
		}
		for _, f := range a.files {
			if rel != "" && !strings.HasPrefix(f.Path, rel+"/") {
				continue
			}
			if len(rel) > depth {
				best, depth = i, len(rel)
			}
			break
		}
	}
	return best
}

func (a *App) chooseTarget(i int) {
	if i < 0 || i >= len(a.sonda.targets) {
		return
	}
	a.sonda.target = i
	a.note("target %s", a.sonda.targets[i].Name)
}

// baselineRev names the revision the change is measured against: the start
// of a range, or the parent of the change.
func (a *App) baselineRev() (rev, why string) {
	switch {
	case a.spec.Kind == vcs.DiffRange:
		return a.spec.From, ""
	case a.rev.ChangeID == "":
		return "", "no change is selected"
	case len(a.rev.Parents) == 0:
		return "", a.rev.ChangeID + " has no parent to measure against"
	}
	return a.rev.Parents[0], ""
}

// startSonda builds and starts both runs: the baseline first, since its
// build has the most to do, and the working copy under review after it.
func (a *App) startSonda() {
	s := a.sonda
	if len(s.targets) == 0 {
		a.note("no target declared in %s", sonda.File)
		return
	}
	if s.preparing {
		a.note("still preparing the baseline")
		return
	}
	a.stopSonda()
	t := s.targets[s.target]
	filter := s.currentFilter()
	base, why := a.baselineRev()
	s.panes[paneBaseline].reset("BASELINE", base)
	s.panes[paneReview].reset("REVIEW", "WORKING COPY")
	s.panes[paneBaseline].built, s.panes[paneReview].built = filter, filter

	review := func() {
		s.panes[paneReview].run = sonda.Start(t, "working copy", a.wake)
	}
	if base == "" {
		s.panes[paneBaseline].note = why
		review()
		a.note("running %s", t.Name)
		return
	}

	dir, err := sonda.WorkspaceDir(a.repo.Root())
	var baseTarget sonda.Target
	if err == nil {
		baseTarget, err = t.Relocate(a.repo.Root(), dir)
	}
	if err != nil {
		s.panes[paneBaseline].note = err.Error()
		a.fail(err)
		review()
		return
	}
	s.preparing = true
	repo := a.repo
	a.backgroundIn(context.Background(), 10*time.Minute, func(ctx context.Context) func() {
		err := repo.Workspace(ctx, dir, base)
		return func() {
			s.preparing = false
			if err != nil {
				s.panes[paneBaseline].note = "cannot check out " + base + ": " + firstLine(err.Error())
				a.fail(err)
				review()
				return
			}
			s.panes[paneBaseline].run = sonda.Start(baseTarget, base, a.wake)
			review()
			a.note("running %s at %s and in the working copy", baseTarget.Name, base)
		}
	})
}

// wake asks for a frame from another goroutine, which is how a run reports
// that there is something new to draw.
func (a *App) wake() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// stopSonda asks both runs to stop. They stay on screen with how they ended.
func (a *App) stopSonda() {
	if a.sonda == nil {
		return
	}
	stopped := 0
	for i := range a.sonda.panes {
		if r := a.sonda.panes[i].run; r != nil && r.State().Phase != sonda.Ended {
			r.Stop()
			stopped++
		}
	}
	if stopped > 0 {
		a.note("stopping")
	}
}

// shutdownSonda stops the runs as the window closes, and waits for them long
// enough for the grace period to run out, so a program that ignores the stop
// is killed rather than orphaned.
func (a *App) shutdownSonda() {
	if a.sonda == nil {
		return
	}
	var runs []*sonda.Run
	for i := range a.sonda.panes {
		if r := a.sonda.panes[i].run; r != nil {
			r.Stop()
			runs = append(runs, r)
		}
	}
	deadline := time.After(sonda.Grace + time.Second)
	for _, r := range runs {
		select {
		case <-r.Done():
		case <-deadline:
			return
		}
	}
}

// resetSonda drops the screen, for a new repository.
func (a *App) resetSonda() {
	a.stopSonda()
	a.sonda = nil
}

func (s *sondaScreen) currentFilter() logFilter {
	return logFilter{min: s.minLevel, text: strings.ToLower(strings.TrimSpace(s.filter.Text()))}
}

// syncSonda brings both panes up to date with their runs, once a frame.
func (a *App) syncSonda() {
	s := a.sonda
	f := s.currentFilter()
	for i := range s.panes {
		s.panes[i].sync(f, s.raw)
	}
}

func (p *logPane) reset(label, rev string) {
	*p = logPane{label: label, rev: rev, follow: true}
	p.list.Axis = layout.Vertical
}

func (p *logPane) sync(f logFilter, raw bool) {
	if p.run == nil {
		return
	}
	p.state = p.run.State()
	var grown []int
	p.logs, grown = p.run.Sync(p.logs)
	p.refilter(f, raw, grown)
	if n := len(p.view); n == 0 {
		p.cursor = 0
	} else if p.follow {
		p.cursor = n - 1
	} else {
		p.cursor = clamp(p.cursor, 0, n-1)
	}
}

// refilter judges what is new against the filter, and everything against a
// filter that changed. A log that grew since it was judged is judged again:
// a line joined to it can only bring it into view, never take it out.
func (p *logPane) refilter(f logFilter, raw bool, grown []int) {
	if f != p.built {
		p.view, p.seen, p.built = p.view[:0], 0, f
		grown = nil
	}
	for _, i := range grown {
		if i >= p.seen {
			continue
		}
		p.measure(p.logs[i], raw)
		if at, shown := slices.BinarySearch(p.view, i); !shown && f.pass(p.logs[i]) {
			p.view = slices.Insert(p.view, at, i)
		}
	}
	for i := p.seen; i < len(p.logs); i++ {
		if f.pass(p.logs[i]) {
			p.view = append(p.view, i)
		}
		p.measure(p.logs[i], raw)
	}
	p.seen = len(p.logs)
}

func (p *logPane) measure(l sonda.Log, raw bool) {
	for _, line := range logLines(l, raw) {
		p.cols = max(p.cols, width(line))
	}
}

// remeasure finds the widest line again, after the way lines are set changed.
func (p *logPane) remeasure(raw bool) {
	p.cols, p.x = 0, 0
	for _, l := range p.logs {
		p.measure(l, raw)
	}
}

// sondaKey is the whole keyboard while the screen is up.
func (a *App) sondaKey(gtx layout.Context, ke key.Event, editing bool) {
	s := a.sonda
	shift := ke.Modifiers.Contain(key.ModShift)
	if editing {
		switch ke.Name {
		case key.NameEscape, key.NameReturn, key.NameEnter:
			s.filter.Defocus(gtx)
			a.revsetInput.Defocus(gtx)
		}
		return
	}
	switch ke.Name {
	case key.NameEscape:
		if a.help {
			a.help = false
			return
		}
		a.toggleSonda()
	case "S":
		a.toggleSonda()
	case key.NameTab, key.NameLeftArrow, key.NameRightArrow, "H":
		s.focus = 1 - s.focus
	case key.NameUpArrow, "K":
		a.sondaMove(func(at, _ int) int { return at - 1 })
	case key.NameDownArrow, "J":
		a.sondaMove(func(at, _ int) int { return at + 1 })
	case key.NameHome:
		a.sondaMove(func(_, _ int) int { return 0 })
	case key.NameEnd:
		a.sondaMove(func(_, n int) int { return n - 1 })
	case "G":
		if shift {
			a.sondaMove(func(_, n int) int { return n - 1 })
		} else {
			a.sondaMove(func(_, _ int) int { return 0 })
		}
	case key.NameSpace, key.NamePageDown, key.NamePageUp:
		step := 1
		if ke.Name == key.NamePageUp || (ke.Name == key.NameSpace && shift) {
			step = -1
		}
		rows := max(1, gtx.Constraints.Max.Y/a.ui.CodeRow(gtx)-4)
		a.sondaMove(func(at, _ int) int { return at + rows*step })
	case "R":
		a.startSonda()
	case "X":
		a.stopSonda()
	case "W":
		a.toggleRaw()
	case "L":
		a.cycleLevel()
	case "/":
		if shift {
			a.help = !a.help
		} else {
			s.filter.Focus(gtx)
		}
	case "T":
		a.toggleDark()
	case ",":
		a.toggleSettings()
	}
}

// sondaMove moves the reading position of the focused pane. Landing on the
// newest line is what makes the pane follow the run again.
func (a *App) sondaMove(where func(at, n int) int) {
	p := &a.sonda.panes[a.sonda.focus]
	n := len(p.view)
	if n == 0 {
		return
	}
	p.cursor = clamp(where(p.cursor, n), 0, n-1)
	p.follow = p.cursor == n-1
	a.scrollList(&p.list, p.cursor)
}

func (a *App) toggleRaw() {
	s := a.sonda
	s.raw = !s.raw
	for i := range s.panes {
		s.panes[i].remeasure(s.raw)
	}
	if s.raw {
		a.note("raw lines")
	} else {
		a.note("formatted")
	}
}

// cycleLevel steps the level filter through the levels worth asking for,
// and back to everything.
func (a *App) cycleLevel() {
	s := a.sonda
	switch s.minLevel {
	case sonda.Unlevelled:
		s.minLevel = sonda.Info
	case sonda.Info:
		s.minLevel = sonda.Warn
	case sonda.Warn:
		s.minLevel = sonda.Error
	default:
		s.minLevel = sonda.Unlevelled
	}
	a.note("level %s", levelLabel(s.minLevel))
}

func levelLabel(min sonda.Severity) string {
	if min == sonda.Unlevelled {
		return "ALL"
	}
	return min.Name() + "+"
}

// sondaTitle names a pane and says where its run is.
func (p *logPane) sondaTitle(filtered bool) string {
	parts := []string{p.label}
	if p.rev != "" {
		parts = append(parts, p.rev)
	}
	if p.run == nil {
		parts = append(parts, "NOT RUN")
	} else {
		switch p.state.Phase {
		case sonda.Building:
			parts = append(parts, "BUILDING")
		case sonda.Running:
			parts = append(parts, "RUNNING")
		default:
			parts = append(parts, strings.ToUpper(p.state.Ended))
		}
		switch {
		case filtered:
			parts = append(parts, fmt.Sprintf("%d/%d", len(p.view), len(p.logs)))
		case len(p.logs) > 0:
			parts = append(parts, fmt.Sprint(len(p.logs)))
		}
	}
	return strings.Join(parts, " · ")
}
