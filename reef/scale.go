package reef

import (
	"gioui.org/font"
	"gioui.org/unit"
)

// The scales: type, weight, rhythm, space and structure. Constants rather than
// Theme fields — an application picks from the scale, it does not redefine it.
// The exception is the interface size, which Theme.SetSize anchors the scale
// on.

// The type scale. Sizes are fixed rather than fluid, so a step means the same
// thing on every screen. The scale is split by job: SizeData and below are set
// in the working face and scanned, SizeBody and above are set as language.
const (
	SizeMicro     unit.Sp = 10
	SizeLabel     unit.Sp = 11
	SizeMeta      unit.Sp = 12
	SizeData      unit.Sp = 13
	SizeBody      unit.Sp = 14
	SizeBodyLg    unit.Sp = 16
	SizeH4        unit.Sp = 16
	SizeH3        unit.Sp = 20
	SizeH2        unit.Sp = 26
	SizeH1        unit.Sp = 34
	SizeDisplay   unit.Sp = 52
	SizeDisplayLg unit.Sp = 76
)

// The roles an interface sets type in.
const (
	SizeUI       = SizeData // lists, fields, everything read as data
	SizeCode     = SizeData // source listings
	SizeSection  = SizeH4
	SizeWordmark = SizeH4
)

// The weights.
const (
	WeightLight   = font.Light  // 300
	WeightBody    = font.Normal // 400
	WeightLabel   = font.Medium // 500, and every heading below display
	WeightDisplay = font.Bold   // 700, display type and a wordmark
)

// Line heights, as multiples of the point size. Rows are laid out on these
// rather than on ad hoc padding, so a list and a listing stay on one rhythm.
const (
	LineTight  = 1.08
	LineSnug   = 1.28
	LineNormal = 1.5
	LineLoose  = 1.65
	LineCode   = 1.55
)

// Tracking, in ems. Uppercase plus caps tracking means "this is a label, not
// content"; nothing else in the system is tracked.
const (
	TrackDisplay = -0.03
	TrackHeading = -0.02
	TrackNormal  = 0
	TrackLabel   = 0.06
	TrackCaps    = 0.12
)

// The 4px spacing scale. Gaps come from these, never from stacked margins.
const (
	Sp0  unit.Dp = 0
	Sp1  unit.Dp = 2
	Sp2  unit.Dp = 4
	Sp3  unit.Dp = 6
	Sp4  unit.Dp = 8
	Sp5  unit.Dp = 12
	Sp6  unit.Dp = 16
	Sp7  unit.Dp = 20
	Sp8  unit.Dp = 24
	Sp9  unit.Dp = 32
	Sp10 unit.Dp = 40
	Sp11 unit.Dp = 56
	Sp12 unit.Dp = 72
	Sp13 unit.Dp = 96
)

// Dense by default: twenty rows without scrolling. GapRow is the standard row
// height.
const (
	GapRow     unit.Dp = 28
	ControlHSm unit.Dp = 22
	ControlH   unit.Dp = 28
	ControlHLg unit.Dp = 36
	PadInline  unit.Dp = 10
	PadCard    unit.Dp = 16
	Gutter     unit.Dp = 24
	TopbarH    unit.Dp = 44
	RailW      unit.Dp = 28
)

// Structure. Everything is square and nothing casts a shadow, so what is left
// is hairline weights and the grid.
const (
	BorderHair  unit.Dp = 1
	BorderThick unit.Dp = 2
	GridDotStep unit.Dp = 8
)

// The range Theme.SetSize will accept. The scale is anchored on SizeUI, so
// choosing 15 makes every step of it 15/13 larger: the interface keeps its
// proportions and only changes how much of the screen it asks for.
const (
	MinFontSize unit.Sp = 9
	MaxFontSize unit.Sp = 24
)
