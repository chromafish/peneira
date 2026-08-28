// Package reef is a design system for Gio, for dense, keyboard-first,
// monospaced interfaces. It ships the tokens, the type scale, the drawing
// primitives and the controls.
//
// # The shape of it
//
// Two values carry the system. A [Theme] is the palette, the shaper the type
// is set with, and the measurements everything is laid out against. A [UI] is a Theme plus the one piece of state controls need between
// frames — which of them the pointer is over — and is what an application
// threads through its layout:
//
//	type App struct {
//		ui *reef.UI
//	}
//
//	func main() {
//		a := &App{ui: reef.New()}
//		go func() {
//			w := new(app.Window)
//			var ops op.Ops
//			for {
//				switch e := w.Event().(type) {
//				case app.DestroyEvent:
//					os.Exit(0)
//				case app.FrameEvent:
//					gtx := app.NewContext(&ops, e)
//					a.Layout(gtx)
//					e.Frame(gtx.Ops)
//				}
//			}
//		}()
//		app.Main()
//	}
//
//	func (a *App) Layout(gtx layout.Context) layout.Dimensions {
//		gtx = a.ui.Sized(gtx)                        // the chosen body size
//		reef.Fill(gtx, gtx.Constraints.Max, a.ui.P.Bg)
//		a.ui.Panel(gtx, reef.Panel{Title: "FILES", Focused: true, Body: a.files})
//		return layout.Dimensions{Size: gtx.Constraints.Max}
//	}
//
// UI embeds *Theme, so a.ui.P, a.ui.Cell and every other theme method are
// reachable through it and there is only one value to carry. One UI belongs to
// one window.
//
// # Colour
//
// Colour comes in two levels. A [Ramp] holds the raw steps — two neutral ramps
// and four accent families — and a [Palette] holds the semantic aliases
// everything is drawn with: [Palette.Bg], [Palette.Fg], [Palette.Rule],
// [Palette.Action], [Palette.Warn], and the rest. [Ramp.Palette] builds the
// second from the first, and both themes the system ships go through it, which is
// what guarantees they stay the same design.
//
// Draw from the aliases, not from a Ramp step. That is what lets an interface
// invert by swapping one struct:
//
//	th.ToggleDark()
//
// An application with its own colours should write a [Ramp] and pass it
// through [Ramp.Palette] rather than filling in a Palette by hand.
//
// The page is a light green-grey rather than white, and a panel sits a step
// lighter than it. Moss green is the brand and every primary action; there
// is no blue in the shipped themes. Pair a status colour with a word or a
// glyph — colour alone is not enough.
//
// # Themes people bring
//
// A [Scheme] is a named palette, or a pair of them for light and dark, which
// is what a settings list offers and what [Theme.SetScheme] puts in force.
// [Builtin] is what ships: [Default], and [BoneScheme] with near-white
// neutrals.
//
// [Base16] imports the sixteen-colour format nearly every editor and terminal
// scheme is already published in, which is how a person gets the colours they
// already use:
//
//	b, err := reef.ParseBase16(file)     // the .yaml or .json as published
//	th.SetScheme(b.Scheme("gruvbox"))
//
// [LoadSchemes] reads a directory of them. An imported scheme knows whether it
// is light or dark and fills in only that mode, so inverting it does nothing;
// [Scheme.Modes] says which it has.
//
// Colours from outside can be legal and still unreadable, so [Palette.Check]
// reports aliases left transparent and pairs that cannot be told apart, with
// [Contrast] behind it. Run it when a scheme is loaded and show what it says.
//
// # Type
//
// The system is set in IBM Plex, embedded in the binary: Plex Mono is the
// working typeface — data, code, labels, controls — and Plex Sans Condensed is
// display only, which in practice means a wordmark. The scale is split by job:
// [SizeData] and below are scanned, [SizeBody] and above are read as language.
// Sizes come from a fixed scale ([SizeLabel], [SizeData], [SizeH4] and so on)
// rather than from a fluid one.
//
// Text is set in runs, not paragraphs. [Run] is one line in one style, and
// [Theme.TextAt], [Theme.TextRight] and [Theme.TextCentred] place a run in a
// band without wrapping it; a run that will not fit is truncated. [Wrap]
// breaks prose into lines, and [Theme.Cell] measures the monospace grid the
// layout is arithmetic on.
//
// [Theme.Label] is the other half: 11px medium, capitals, tracked wide.
// Uppercase plus tracking marks a label; content keeps sentence case.
//
// A person may set the interface size the whole scale is anchored on, with
// [Theme.SetSize], and the typeface with [Theme.SetFamily]; [MonoFamilies]
// lists what there is to choose from. [Theme.Sized] is what applies the chosen
// size, once, at the top of a window's layout.
//
// # Structure
//
// Structure is filled rectangles and hairlines. No radius, no gradient, no
// shadow: something that floats is drawn on the raised surface behind an ink
// hairline. [Fill], [FillRect], [HLine], [VLine], [Edge], [Outline] and
// [Stroke] are the set; [Theme.Sheet] is what floats; [Theme.DotGrid] means
// "nothing here".
//
// # Controls
//
// Controls print their own keyboard shortcut, so the keyboard is learned from
// the mouse.
//
//	x += ui.Control(gtx, x, height, tagSave, "SAVE  ⌘S", ui.P.Action, a.save)
//
// [UI.Control], [UI.Box], [UI.Collapse], [UI.Rail], [Panel], [Field] and
// [Splits] are the set. They draw immediately and identify themselves with
// tags — any comparable value that is stable between frames — so a row of a
// list can be a control without keeping a widget per row.
//
// # Layout
//
// The package does not lay anything out for you. It draws into the space it is
// given and returns how wide or tall it was; the application composes those
// numbers with offsets. The interfaces reef is for are grids of fixed rows
// measured in character cells, where flex and stack layouts are a longer way
// round to the same pixels.
//
// [Theme.Row] is the pitch of a list row, [Theme.CodeRow] a line of a source
// listing, [Theme.TextRow] a line of running text, and [Theme.StripHeight] a
// header or status strip. Lay rows out on those and every list in an
// application prints on the same rhythm.
package reef
