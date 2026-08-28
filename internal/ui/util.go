package ui

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
)

// clamp keeps an index or a pixel measurement inside a range. It is used
// wherever the application moves a selection or a scroll position: the bounds
// are the list, and going past them is not an error worth reporting.
func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

// The interface is laid out by hand rather than with flex and stack widgets,
// so drawing anywhere but the origin means offsetting, constraining a copy of
// the context, and putting the origin back.

// fill draws into an exact box of size, with the origin moved to p. What is
// drawn takes the whole box.
func fill(gtx layout.Context, p, size image.Point, draw func(layout.Context)) {
	off := op.Offset(p).Push(gtx.Ops)
	gtx.Constraints = layout.Exact(size)
	draw(gtx)
	off.Pop()
}

// fit is fill for text, which measures itself: size is an upper bound, so a
// short run stays short and a long one is truncated at the edge of the box.
func fit(gtx layout.Context, p, size image.Point, draw func(layout.Context)) {
	off := op.Offset(p).Push(gtx.Ops)
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max = size
	draw(gtx)
	off.Pop()
}

// thousands groups a number for reading: 384102 as 384,102. It is used where a
// figure is large enough that the digits alone do not say how large.
func thousands(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// firstLine is the first line of a possibly multi-line string.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
