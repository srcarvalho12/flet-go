# Desktop apps

```go
err := flet.App(main)
```

`App` starts a local server, launches the Flet desktop client and runs `main` when
it connects. It returns when the window is closed. Pressing Ctrl+C closes the
window too.

## Options

```go
flet.App(main, flet.AppOptions{AssetsDir: "assets"})
```

| Option | Purpose |
|---|---|
| `AssetsDir` | Folder with your images, fonts and icons (`&flet.Image{Src: "logo.png"}`) |

## The client

The Flet desktop client is downloaded from the official Flet release on the first
run and cached:

| OS | Cache folder |
|---|---|
| macOS | `~/Library/Caches/flet-go/client/v1.1.2` |
| Windows | `%LocalAppData%\flet-go\client\v1.1.2` |
| Linux | `~/.cache/flet-go/client/v1.1.2` |

Automatic download works on macOS and Windows. On Linux, download the client
yourself and set `FLET_CLIENT_PATH`.

| Variable | Purpose |
|---|---|
| `FLET_CLIENT_PATH` | Use an installed client (folder, `.app` or executable) |
| `FLET_CLIENT_URL` | Download the client from another URL |
| `FLET_ASSETS_DIR` | Same as `AssetsDir` |
| `FLETGO_DEBUG=1` | Log the messages and the client output |

## The window

`page.Window` has the window size, position and state, and methods such as
`Center`, `Close`, `ToFront` and `StartDragging`:

```go
page.Window.Width, page.Window.Height = 900, 600
page.Update()
page.Window.Center()
```

## Reconnection

If the connection drops while the window is open, the client reconnects and gets
the same page back. `main` does not run again.
