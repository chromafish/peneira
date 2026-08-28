package reef

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// UI is the design system with a pointer in it: a Theme, plus the one piece of
// state the controls need between frames — which of them the pointer is over.
// Every control in the package hangs off it, and an application threads one
// through its layout the way it would thread a theme.
//
//	type App struct {
//		ui *reef.UI
//	}
//
//	a := &App{ui: reef.New()}
//	...
//	a.ui.Control(gtx, x, h, tagSave, "SAVE  ⌘S", a.ui.P.Action, a.save)
//
// UI embeds *Theme, so a.ui.P, a.ui.Cell and every other theme method are
// reachable through it and there is only one value to carry. One UI belongs to
// one window: only a single control is under the pointer at a time, which is
// why the state is a single tag rather than a set.
type UI struct {
	*Theme
	hover event.Tag
}

// New returns a UI on a fresh theme in the embedded faces.
func New() *UI { return NewUI(NewTheme()) }

// NewUI returns a UI on a theme of your own.
func NewUI(t *Theme) *UI { return &UI{Theme: t} }

// Redraw asks for another frame.
//
// A pointer handler runs part way through the frame it belongs to: the layout
// around it has already been measured, so whatever the handler changes cannot
// appear until the next frame, and nothing else will ask for that frame. The
// change would otherwise sit unseen until something unrelated caused a redraw.
// Every handler in this package ends with one of these.
func Redraw(gtx layout.Context) { gtx.Execute(op.InvalidateCmd{}) }

// Hovered reports whether the control with this tag is under the pointer.
// Draw the hover state from it — the system tints one paper step darker, on
// the frame it happens, so it reads as the cursor rather than as an animation.
//
// A tag is any comparable value that identifies the control and stays the same
// between frames: a pointer to a field for a control an application has one
// of, or a small struct for one of many, such as a row of a list.
//
//	type rowTag struct{ row int }
func (u *UI) Hovered(tag event.Tag) bool { return u.hover != nil && u.hover == tag }

// Area registers a rectangle in the current coordinate space that tracks the
// pointer and runs onClick when it is pressed. The cursor changes over it —
// pass pointer.CursorPointer for a control, pointer.CursorText for something
// selectable, or pointer.CursorDefault to leave it alone.
//
// onClick may be nil, for a region that only wants the hover.
func (u *UI) Area(gtx layout.Context, r image.Rectangle, tag event.Tag, cursor pointer.Cursor, onClick func()) {
	stack := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	if cursor != pointer.CursorDefault {
		cursor.Add(gtx.Ops)
	}
	stack.Pop()
	u.events(gtx, tag, onClick)
}

// Click is Area over the whole of a widget's own space, with no cursor of its
// own: the shape a row of a list takes.
func (u *UI) Click(gtx layout.Context, size image.Point, tag event.Tag, onClick func()) {
	u.Area(gtx, image.Rectangle{Max: size}, tag, pointer.CursorDefault, onClick)
}

// events drains the pointer events for one tag, keeping the hover current.
func (u *UI) events(gtx layout.Context, tag event.Tag, onClick func()) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: tag,
			Kinds:  pointer.Press | pointer.Enter | pointer.Leave,
		})
		if !ok {
			return
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Enter:
			u.hover = tag
		case pointer.Leave:
			if u.hover == tag {
				u.hover = nil
			}
		case pointer.Press:
			if onClick != nil {
				onClick()
			}
		default:
			continue
		}
		Redraw(gtx)
	}
}

// SetHover puts the pointer on a tag by hand. The router does this from real
// pointer events, so an application has no reason to call it — it is here for
// specimens and tests, which have to draw a control in its hovered state
// without a pointer to hover it with.
func (u *UI) SetHover(tag event.Tag) { u.hover = tag }

// Release drops the hover, for the case where the control under the pointer
// has gone away without the pointer having left it — a sheet closing, a list
// rebuilding under the cursor.
func (u *UI) Release() { u.hover = nil }
