package reef

import (
	"image"

	"gioui.org/font"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// Field is a single-line or wrapping text input, drawn as a sunken well with a
// hairline round it rather than as a bordered box on the page. It wraps Gio's
// editor and adds the two things a keyboard-first interface needs from one:
// the focus state as of the last frame, so the application can tell a command
// key from a typed character, and a placeholder.
//
//	f := reef.NewField("")
//	f.Placeholder = "(anything)"
//	...
//	text, submitted := f.Update(gtx)   // once a frame, before the keys
//	if submitted { ... }
//	...
//	f.Layout(gtx, th)                  // in the space it should fill
type Field struct {
	editor  widget.Editor
	focused bool

	// Placeholder is shown, dimmed, while the field is empty.
	Placeholder string
}

// NewField returns a single-line field containing text. Return submits it,
// which Update reports.
func NewField(text string) *Field {
	f := &Field{}
	f.editor.SingleLine = true
	f.editor.Submit = true
	f.editor.SetText(text)
	return f
}

// NewMultiField returns a wrapping field for prose. Return inserts a newline
// rather than submitting, so what is typed can have paragraphs; save it on a
// modified Return instead.
func NewMultiField(text string) *Field {
	f := &Field{}
	f.editor.SetText(text)
	f.editor.SetCaret(len(text), len(text))
	return f
}

// Editor exposes the underlying editor, for the things this wrapper does not
// cover — a selection, a caret position, a filter on what may be typed.
func (f *Field) Editor() *widget.Editor { return &f.editor }

// Text returns the current contents.
func (f *Field) Text() string { return f.editor.Text() }

// SetText replaces the contents, leaving the caret at the end.
func (f *Field) SetText(s string) {
	f.editor.SetText(s)
	f.editor.SetCaret(len(s), len(s))
}

// Focused reports whether the field had keyboard focus as of the last frame,
// which is how an application tells a command key from a typed character.
func (f *Field) Focused() bool { return f.focused }

// Focus moves keyboard focus to the field and selects everything in it.
func (f *Field) Focus(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: &f.editor})
	f.editor.SetCaret(0, len(f.editor.Text()))
}

// Defocus moves keyboard focus away from the field.
func (f *Field) Defocus(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: nil})
}

// Update drains the field's events and reports its contents, and whether
// Return was pressed. Call it once a frame, before deciding what any other
// keystroke means.
func (f *Field) Update(gtx layout.Context) (text string, submitted bool) {
	for {
		ev, ok := f.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	f.focused = gtx.Focused(&f.editor)
	return f.editor.Text(), submitted
}

// Layout draws the field into the space it is given, filling it.
func (f *Field) Layout(gtx layout.Context, t *Theme) layout.Dimensions {
	size := gtx.Constraints.Max
	Fill(gtx, size, t.P.BgSunken)

	// A well is bounded by a hairline; focus is the 2px ring the system puts
	// on everything reachable from the keyboard, and it is never hidden.
	box := image.Rect(0, 0, size.X, size.Y)
	if f.focused {
		t.FocusRing(gtx, box.Inset(gtx.Dp(BorderHair)))
	} else {
		Outline(gtx, box, gtx.Dp(BorderHair), t.P.Rule)
	}

	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: t.P.Fg}.Add(gtx.Ops)
	textMaterial := macro.Stop()

	macro = op.Record(gtx.Ops)
	paint.ColorOp{Color: t.P.TextSel}.Add(gtx.Ops)
	selectMaterial := macro.Stop()

	pad := gtx.Dp(Sp3)
	cell := t.Cell(gtx, SizeUI, false)
	inner := size.Y
	top := 0
	if f.editor.SingleLine {
		inner = cell.Y
		top = (size.Y - cell.Y) / 2
	}
	off := op.Offset(image.Pt(pad, top)).Push(gtx.Ops)
	sub := gtx
	sub.Constraints.Min = image.Pt(size.X-2*pad, 0)
	sub.Constraints.Max = image.Pt(size.X-2*pad, inner)
	if f.Placeholder != "" && f.editor.Len() == 0 {
		t.Text(sub, Body(t.P.Faint), f.Placeholder)
	}
	f.editor.Layout(sub, t.Shaper, t.Font(WeightBody, font.Regular), SizeUI, textMaterial, selectMaterial)
	off.Pop()
	return layout.Dimensions{Size: size}
}

// FieldHeight is how tall a single-line field should be: deep enough for its
// own line of text, and never shallower than a small control. A field that
// cannot hold its own line of text is a field whose contents cannot be read.
func (t *Theme) FieldHeight(gtx layout.Context) int {
	return max(gtx.Dp(ControlHSm), t.Cell(gtx, SizeUI, false).Y+gtx.Dp(Sp2))
}
