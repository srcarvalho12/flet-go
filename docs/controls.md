# Controls, events and methods

All Flet controls are generated from the Flet Python SDK, so the
[Flet documentation](https://flet.dev/docs/controls) applies: the names change
from `snake_case` to Go's `CamelCase`.

| Flet (Python) | flet-go |
|---|---|
| `ft.Text("Hi", size=20)` | `&flet.Text{Value: "Hi", Size: 20}` |
| `ft.Container(content=..., bgcolor=ft.Colors.BLUE)` | `&flet.Container{Content: ..., Bgcolor: colors.Blue}` |
| `ft.Icon(ft.Icons.FAVORITE)` | `&flet.Icon{Icon: icons.Favorite}` |
| `on_click=handler` | `OnClick: handler` |

## Fields

The zero value of a field means "not set": the client uses Flet's default.

Some fields default to something other than zero (for example `Visible` defaults to
`true`). Those are pointers, so "not set" and "false" are different. Use the
helpers:

```go
&flet.Text{Value: "hidden", Visible: flet.Bool(false)}
&flet.Slider{Max: flet.Float(100)}
```

`flet.Bool`, `flet.Float`, `flet.Int` and `flet.Str` return pointers.

### Common types

| Flet type | Go type | Example |
|---|---|---|
| Enums | Named constants | `flet.FontWeightBold`, `flet.MainAxisAlignmentCenter` |
| Colors | `flet.Color` | `colors.Red`, `"#ff0000"`, `colors.Blue.WithOpacity(0.5)` |
| Icons | `flet.IconData` | `icons.Add`, `icons.CupertinoHeart` |
| Durations | `time.Duration` | `500 * time.Millisecond` |
| Styles and other values | Pointers to structs | `&flet.TextStyle{Size: 14}` |
| A child control | `flet.Control` | `Content: &flet.Text{...}` |
| A list of children | `[]flet.Control` | `Controls: []flet.Control{...}` |
| Text or a control | `any` | `Content: "OK"` or `Content: &flet.Row{...}` |
| Unions (padding, margin...) | `any` | `Padding: 16` or `Padding: &flet.Padding{Left: 8}` |

Colors are in the `colors` package and icons in the `icons` package:

```go
import (
	"github.com/srcarvalho12/flet-go/colors"
	"github.com/srcarvalho12/flet-go/icons"
)
```

## Updating the screen

Change fields and call `page.Update()`. You can change anything: values, styles,
or the children lists (add, remove, reorder). flet-go compares with what the client
already has and sends only the difference.

```go
list.Controls = append(list.Controls, &flet.Text{Value: "new item"})
list.Controls[0].(*flet.Text).Value = "changed"
page.Update()
```

`page.Add`, `page.Remove` and `page.Clean` change the page's first view and update
it.

When the user types or moves a slider, the control's field is updated before your
handler runs. Those changes are not sent back to the client.

## Events

Handlers are fields named `On...`. Most receive a `*flet.Event`:

```go
&flet.Button{Content: "Save", OnClick: func(e *flet.Event) {
	// e.Control is the button, e.Page is the page
}}
```

Events with extra data have their own type:

```go
&flet.GestureDetector{OnTap: func(e *flet.TapEvent) {
	fmt.Println(e.LocalPosition)
}}
```

Handlers of a page run one at a time, in the order the user acted. They may call
client methods and wait for the result (see below).

## Client methods

Controls with client-side actions have Go methods. They block until the client
answers and return an error:

```go
err := field.Focus()
err = list.ScrollTo(flet.ScrollToOptions{Offset: flet.Float(0)})
err = page.Window.Center()
```

Required arguments come first. Optional ones go in an `...Options` struct:

```go
files, err := picker.PickFiles(flet.PickFilesOptions{AllowMultiple: true})
```

A method on a control that is not on the page returns `flet.ErrNotMounted`. An
error reported by the client is a `*flet.MethodError`. If the client disconnects
while waiting, the call returns `flet.ErrDisconnected`.

## What is not generated yet

- Canvas shapes (`flet.canvas`).
- Extension packages (charts, maps, video, audio...): they need a custom client.
- A few fields with rare types (dates and times).
- `Share` methods and some page methods (`get_device_info`...).
