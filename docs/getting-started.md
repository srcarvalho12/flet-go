# Getting started

## Install

```bash
go mod init myapp
go get github.com/srcarvalho12/flet-go
```

flet-go needs Go 1.27 or newer. You do not need Flutter, Python or Node.js.

## Your first app

Create `main.go`:

```go
package main

import (
	"log"

	flet "github.com/srcarvalho12/flet-go"
)

func main() {
	err := flet.App(func(page *flet.Page) {
		page.Title = "Hello"
		greeting := &flet.Text{Value: "Type your name"}
		name := &flet.TextField{Label: "Name"}

		page.Add(
			name,
			&flet.Button{Content: "Say hello", OnClick: func(e *flet.Event) {
				greeting.Value = "Hello, " + name.Value + "!"
				page.Update()
			}},
			greeting,
		)
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

Run it:

```bash
go run .
```

On the first run, the Flet desktop client is downloaded and cached (about 55 MB on
macOS). Later runs open the window right away.

## The three ideas

1. **Controls are structs.** `&flet.Text{Value: "Hi", Size: 20}`. Fields you do
   not set keep the Flet default.
2. **Change fields, then call `page.Update()`.** Only the difference is sent to
   the client.
3. **Handlers are Go functions.** `OnClick`, `OnChange`... run in Go when the user
   acts. Values typed by the user (like `name.Value` above) are already in the
   struct when the handler runs.

## Run it in the browser

Replace `flet.App(main)` with `flet.Serve(":8550", main)`:

```go
err := flet.Serve(":8550", func(page *flet.Page) {
	// same code
})
```

Open <http://localhost:8550>. Each browser tab gets its own page. The web client is
downloaded once and cached. See [Web apps](web.md).

## Next steps

- Browse the controls in [Controls, events and methods](controls.md).
- Split a bigger app into pages with the [router](routing.md).
- Run the [examples](../examples):

| Example | Shows |
|---|---|
| `counter` | The smallest app |
| `list` | Adding, removing and reordering items |
| `gallery` | Many controls, styles, colors and icons |
| `services` | Dialogs, snack bar, focus, clipboard, file picker, window |
| `web` | The counter in the browser |
| `upload` | File uploads under a base path |
| `router` | Pages in separate files, layouts, login, stacked screens |

```bash
go run ./examples/gallery
```

## Debugging

Set `FLETGO_DEBUG=1` to log every message exchanged with the client:

```bash
FLETGO_DEBUG=1 go run .
```

A panic in your code is logged with its stack trace and shown in the app window.
