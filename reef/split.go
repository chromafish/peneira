package reef

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
)

// Splits divides a window into columns: a fraction of the width for each of
// the leading columns, and the remainder for the last one, which is the column
// the work is in. Fractions rather than pixels, so the layout keeps its
// proportions when the window is resized.
//
//	s := reef.NewSplits(0.22, 0.23)      // two columns, then the rest
//	w := s.Widths(gtx, size.X)           // pixels for the two
//	... draw the columns and the rules between them ...
//	s.Handles(gtx, size, w)              // make the rules draggable
//
// A column can also be put away entirely, to give the last one the window. It
// is never silently gone: a put-away column is drawn as a rail — see UI.Rail —
// which says what it is and brings it back.
//
// Splits carries the state of a drag between frames, so keep one and reuse it;
// do not copy it after use.
type Splits struct {
	// Min is the narrowest a sized column may be drawn, below which it stops
	// being useful. MinLast is the same for the remainder.
	Min     unit.Dp
	MinLast unit.Dp
	// Rail is the width a put-away column leaves behind.
	Rail unit.Dp

	// The fractions a drag may set a column to. They are wide bounds: Min is
	// what actually stops a column collapsing, and a person who wants a column
	// gone can put it away outright.
	MinFraction float32
	MaxFraction float32

	fractions []float32
	hidden    []bool
	dragging  int // 1-based index of the rule being dragged, 0 when none
	handles   []int
}

// The defaults, which suit a window of two indexes and a body.
const (
	defaultMinColumn = unit.Dp(140)
	defaultMinLast   = unit.Dp(260)
)

// NewSplits returns splits with one fraction per leading column; whatever is
// left over belongs to the last column, which is not named here.
func NewSplits(fractions ...float32) *Splits {
	s := &Splits{
		Min:         defaultMinColumn,
		MinLast:     defaultMinLast,
		Rail:        RailW,
		MinFraction: 0.08,
		MaxFraction: 0.7,
		fractions:   append([]float32{}, fractions...),
		hidden:      make([]bool, len(fractions)),
		handles:     make([]int, len(fractions)),
	}
	return s
}

// Columns is how many leading columns there are; the remainder makes one more.
func (s *Splits) Columns() int { return len(s.fractions) }

// Fraction returns the share of the width column i is set to.
func (s *Splits) Fraction(i int) float32 {
	if i < 0 || i >= len(s.fractions) {
		return 0
	}
	return s.fractions[i]
}

// SetFraction sets column i's share, clamped to the allowed range.
func (s *Splits) SetFraction(i int, f float32) {
	if i < 0 || i >= len(s.fractions) {
		return
	}
	s.fractions[i] = clampF(f, s.MinFraction, s.MaxFraction)
}

// Hidden reports whether a column is put away, so the caller can draw a rail
// instead of the column and leave its rule undraggable.
func (s *Splits) Hidden(i int) bool {
	return i >= 0 && i < len(s.hidden) && s.hidden[i]
}

// SetHidden puts a column away, or brings it back.
func (s *Splits) SetHidden(i int, hidden bool) {
	if i >= 0 && i < len(s.hidden) {
		s.hidden[i] = hidden
	}
}

// Toggle puts a column away if it is showing, and back if it is not.
func (s *Splits) Toggle(i int) { s.SetHidden(i, !s.Hidden(i)) }

// Widths converts the stored fractions into pixel widths for the leading
// columns, leaving the rest of total to the last one. A put-away column comes
// back as the width of a rail.
//
// The returned slice is freshly allocated and the caller may keep it.
func (s *Splits) Widths(gtx layout.Context, total int) []int {
	minCol := gtx.Dp(s.Min)
	minLast := gtx.Dp(s.MinLast)
	rail := gtx.Dp(s.Rail)

	out := make([]int, len(s.fractions))
	sum, shown := 0, 0
	for i, f := range s.fractions {
		if s.hidden[i] {
			out[i] = rail
			sum += rail
			continue
		}
		out[i] = clamp(int(float32(total)*f), minCol, total)
		sum += out[i]
		shown++
	}
	if shown == 0 {
		return out
	}

	// Shrink the widest column that is still showing until the last one has
	// the room it needs, and never shrink a rail.
	for over := sum + minLast - total; over > 0; over = sum + minLast - total {
		widest, at := 0, -1
		for i, w := range out {
			if !s.hidden[i] && w > widest {
				widest, at = w, i
			}
		}
		if at < 0 || out[at] <= minCol {
			break
		}
		was := out[at]
		out[at] = max(minCol, was-over)
		sum -= was - out[at]
	}
	return out
}

// Handles registers the drag areas over the rules between the columns and
// applies any movement. The rules themselves are drawn by the caller — a
// hairline at the right edge of each column — and this adds the slightly wider
// invisible strip that is comfortable to grab.
//
// Pass the widths Widths returned for this frame.
func (s *Splits) Handles(gtx layout.Context, size image.Point, widths []int) {
	x := 0
	for i := range widths {
		x += widths[i]
		if i >= len(s.handles) {
			break
		}
		s.handles[i] = x
		x++ // the hairline the caller drew
		if s.Hidden(i) {
			continue
		}
		s.handle(gtx, size, i, widths)
	}
}

func (s *Splits) handle(gtx layout.Context, size image.Point, i int, widths []int) {
	grab := gtx.Dp(Sp2)
	tag := &s.handles[i]
	pos := s.handles[i]
	area := image.Rect(pos-grab, 0, pos+grab+1, size.Y)

	stack := clip.Rect(area).Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	pointer.CursorColResize.Add(gtx.Ops)
	stack.Pop()

	// Where this column starts, so a drag can be measured from it.
	left := 0
	for j := 0; j < i; j++ {
		left += widths[j] + 1
	}

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: tag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			return
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Press:
			s.dragging = i + 1
		case pointer.Release, pointer.Cancel:
			s.dragging = 0
		case pointer.Drag:
			if s.dragging != i+1 || size.X == 0 {
				continue
			}
			// A clip area does not shift the coordinate space, so the position
			// is already in the same space as the handle.
			s.SetFraction(i, float32(int(pe.Position.X)-left)/float32(size.X))
			Redraw(gtx)
		}
	}
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

func clampF(v, lo, hi float32) float32 { return min(max(v, lo), hi) }
