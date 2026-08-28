package ui

import (
	"gioui.org/font"
	"gioui.org/layout"

	"github.com/chromafish/peneira/reef"
)

// Local names for the design system's text helpers, bound to the two sizes
// this interface sets type in: SizeUI for lists and fields, SizeCode for the
// diff body. Each draws one run, vertically centred in a row of the given
// height, and returns its width.

func (a *App) cellText(gtx layout.Context, x, height, limit int, weight font.Weight, c reef.ColorNRGBA, txt string) int {
	return a.ui.TextAt(gtx, reef.Run{Size: reef.SizeUI, Weight: weight, Color: c}, x, height, limit, txt)
}

func (a *App) cellTextRight(gtx layout.Context, rightX, height int, weight font.Weight, c reef.ColorNRGBA, txt string) int {
	return a.ui.TextRight(gtx, reef.Run{Size: reef.SizeUI, Weight: weight, Color: c}, rightX, height, txt)
}

func (a *App) codeText(gtx layout.Context, x, row, limit int, weight font.Weight, c reef.ColorNRGBA, txt string) int {
	return a.codeRun(gtx, x, row, limit, weight, font.Regular, c, txt)
}

// codeStyled is codeText with a slant, used to set comments in italics.
func (a *App) codeStyled(gtx layout.Context, x, row, limit int, style font.Style, c reef.ColorNRGBA, txt string) int {
	return a.codeRun(gtx, x, row, limit, font.Normal, style, c, txt)
}

func (a *App) codeRun(gtx layout.Context, x, row, limit int, weight font.Weight, style font.Style, c reef.ColorNRGBA, txt string) int {
	return a.ui.TextAt(gtx, reef.Run{Size: reef.SizeCode, Weight: weight, Style: style, Color: c}, x, row, limit, txt)
}

func (a *App) codeTextRight(gtx layout.Context, rightX, row int, c reef.ColorNRGBA, txt string) {
	a.ui.TextRight(gtx, reef.Code(c), rightX, row, txt)
}

func (a *App) placeholder(gtx layout.Context, txt string) {
	a.ui.Placeholder(gtx, txt)
}
