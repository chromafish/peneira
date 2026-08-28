# reef

A component library for [Gio](https://gioui.org). Dense, keyboard-first,
monospaced interfaces: filled rectangles and hairlines, no radius, no gradient,
no shadow.

## Theme and UI

Two values carry the system.

A `Theme` is the palette, the shaper the type is set with, and the measurements
everything is laid out against. A `UI` is a `Theme` plus the one piece of state
controls need between frames — which of them the pointer is over — and is what
an application threads through its layout. `UI` embeds `*Theme`, so `ui.P`,
`ui.Cell` and every theme method are reachable through it.

```go
a := &App{ui: reef.New()}

func (a *App) Layout(gtx layout.Context) layout.Dimensions {
	gtx = a.ui.Sized(gtx)
	reef.Fill(gtx, gtx.Constraints.Max, a.ui.P.Bg)
	a.ui.Panel(gtx, reef.Panel{Title: "FILES", Focused: true, Body: a.files})
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
```

## Colour

Colour comes in two levels. A `Ramp` holds the raw steps; a `Palette` holds the
semantic aliases everything is drawn with. `Ramp.Palette()` builds the second
from the first, and every shipped theme goes through it, which is what keeps
them the same design. Draw from the aliases, never from a ramp step — that is
what lets an interface invert with `ToggleDark()`.

`Page` is the page colour, `Raised` is what a panel or a floating sheet sits
on, one step lighter, and the paper steps run from there. Three themes ship —
`Light`, `Bone` with near-white neutrals, and `Dark` — and `Builtin()` is the
schemes a picker offers.

To ship your own colours, write a `Ramp` and pass it through `Ramp.Palette()`
rather than filling in a `Palette` by hand.

A `Scheme` is a named palette, or a pair for light and dark. `Base16` imports
the sixteen-colour format most editor and terminal schemes are already
published in:

```go
b, err := reef.ParseBase16(file)   // the .yaml or .json as published
th.SetScheme(b.Scheme("gruvbox"))
```

`LoadSchemes` reads a directory of them. Colours from outside can be legal and
still unreadable, so `Palette.Check()` reports aliases left transparent and
pairs that cannot be told apart.

## Specimen

```
go run ./reef/cmd/specimen                       # window; t inverts, b papers
go run ./reef/cmd/specimen -png out              # out-light.png, out-dark.png
go run ./reef/cmd/specimen -scheme gruvbox.yaml  # someone else's colours
```

Every alias, every size, every control in each of its states, in every theme.
The colour grid reflects over `Palette`, so a new alias turns up without anyone
adding it.

## Licence

The IBM Plex faces in `plex/` are licensed under the SIL Open Font License 1.1;
`License()` returns the text for a colophon, and shipping the faces means
shipping it with them.
