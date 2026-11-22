# Flet for Go — Plan

Goal: a library where the UI is written in **Go** and rendered by a **Flutter client**, the same way [Flet](https://github.com/flet-dev/flet) does with Python.

---

## 1. How it works

```
┌──────────────────────┐       messages       ┌──────────────────────┐
│       Go side        │  ─── add/update ───▶ │    Flutter client    │
│     control tree     │                      │   renders widgets    │
│   diff + handlers    │  ◀──── event ─────── │     sends events     │
└──────────────────────┘  WebSocket / socket  └──────────────────────┘
```

- Go builds a **control tree** (Text, Button, Column…) in memory.
- The Flutter client only knows how to render that tree. It is a rich "graphical terminal".
- **Go → Flutter:** add, change a property, remove.
- **Flutter → Go:** "button X was clicked", "field Y changed".
- All logic and state live in Go.

Session flow:

1. Go starts a local server.
2. Go launches the client and passes it the address.
3. The client connects and receives the initial tree.
4. On each event, Go runs the handler and sends **only what changed**.

---

## 2. Decision: which Flutter client

> ✅ **Decided: option A** — reuse the Flet client, with the protocol isolated in its own layer.

| | A. Reuse the Flet client | B. Own client |
|---|---|---|
| MVP speed | Fast | Slower |
| Ready-made widgets | Many | Only the ones we build |
| Needs Flutter now | No | Yes |
| Protocol | Flet's (internal, changes between versions) | Ours, simple and stable |
| Project control | Bound to Flet's decisions | Full |
| Work per widget | Go side only | Go + Dart |

Starting with **A** validates the idea quickly. Keeping the protocol in its own layer allows switching to **B** later without rewriting the API developers use.

---

## 3. Go side (the library)

- Each control is a **struct** (Text, Button, Column…) with a **unique ID**.
- The **Page** holds the current tree.
- **Page update:** compares with the last state sent and sends only the difference.
- **Handlers:** a map of "ID + event → function" to dispatch what comes from the client.
- **One goroutine per session/connection.**
- **Separate layers:**
  - **API:** controls and Page. This is what developers use.
  - **Protocol:** turns the tree into messages.
  - **Transport:** WebSocket or local socket.

### Imperative and declarative styles

> ✅ **Decided: imperative only for now.** Declarative mode is a possible later addition.

- **Imperative (current):** create controls, change their fields, call `page.Update()`.
- **Declarative (Flet's components, `use_state`, `use_effect`, `observable`, router):** in Flet it lives entirely in the Python SDK; the client receives the same messages. It can be added in Go without touching the client.

What declarative mode would look like:

```go
func Counter() flet.Control {
	count, setCount := flet.UseState(0)
	return &flet.Row{Controls: []flet.Control{
		&flet.IconButton{Icon: icons.Remove, OnClick: func(*flet.Event) { setCount(count - 1) }},
		&flet.Text{Value: strconv.Itoa(count), Size: 40},
		&flet.IconButton{Icon: icons.Add, OnClick: func(*flet.Event) { setCount(count + 1) }},
	}}
}

flet.App(func(page *flet.Page) { page.Render(Counter) })
```

What it needs:
- **Reconciliation by position and type:** today the diff matches controls by identity. A re-render creates new structs, so ids must be carried over from the previous render, or every change would replace the whole subtree.
- **Hooks:** `UseState`, `UseEffect`, `UseMemo`, `UseRef`, `UseContext` with generics, tracked by call order (like React).
- **Components:** functions returning a control; Go has no decorators, so something like `flet.Component(Counter, props)`.
- The declarative router would be a separate step.

---

## 4. Protocol

- We follow the Flet client protocol, encoded with **MessagePack**.
- Library: **`github.com/vmihailenco/msgpack/v5`** — mature, popular, API similar to `encoding/json`.
- Pinned client version: **v1.1.2**.
- Details in section 10.

---

## 5. Platforms

### Web
- **Server-backed web only**: the Go binary serves the Flutter web client over WebSocket.
- **No static web.**

### Desktop
- Go launches the client and talks to it over a local socket.

### Mobile
- Go is compiled as a **library**: `.so` on Android and `.xcframework` on iOS.
- When the app opens, Flutter calls `Start()`, which starts the Go server in a goroutine.
- The client connects over the local socket, just like on desktop.
- Reason: iOS does not allow an app to spawn another process.

---

## 6. Risks

- **Web latency:** every click is a round trip to the server. Animations and dragging should stay on the Flutter side.
- **Memory per session:** each user keeps a live tree on the server.
- **Reconnection:** connection drops must be handled without losing state.
- **Widget mapping:** the bulk of the work. Each control has its own properties, types and events.
- **Flet protocol:** it is internal and may change between versions.
- **Mobile builds:** Xcode, NDK and code signing break on environment details.

---

## 7. Discarded alternatives

| Library | Why not |
|---|---|
| **Fyne** | Pure Go, but its look and widget variety lag behind Flutter |
| **Gio** | Flexible but low level; much more work to reach the same result |

No WebAssembly or `syscall/js` needed.

---

## 8. Phases and estimates

Revised after studying the Flet source and building the prototype.

| Phase | Deliverable | Estimate |
|---|---|---|
| 1. Prototype | Counter working end to end | ✅ Done |
| 2. Controls | Core widget set (code generator) | ✅ Done |
| 3. Methods, dialogs, services, reconnection | Client calls and a page that survives a dropped connection | ✅ Done |
| 4. Server-backed web | Go binary serving the app | ✅ Done |
| 5–6. Android + iOS | App running on device | 1 to 3 weeks |
| 7. CLI and packaging | Something like `flet-go build` | 2 to 4 days |
| Later (optional). Declarative mode | Components and hooks (see section 3) | 2 to 4 days |

- **Desktop + web:** 1 to 2 weeks.
- **Everything, including mobile:** 3 to 6 weeks.

---

## 9. Prototype (phase 1)

The counter: a text and a "+1" button. Each click increments the number.

It proves:
- Connection to the client.
- Sending the control tree.
- Receiving events.
- Sending only what changed.

---

## 10. Prototype results

✅ **Works** with Flet client v1.1.2 on macOS. The counter opens, and each click sends only the text change.

### Flet client protocol (v1.1.2)

- **Transport:** local TCP (`tcp://127.0.0.1:port`) or Unix socket. The address is the client's first argument.
- **Packet:** `[length:u32 LE][type:u8][body]`
  - Type `0x00`: MessagePack message `[action, body]`.
  - Type `0x01`: binary data channel (not used yet).
- **Actions:**

| Code | Action | Direction |
|---|---|---|
| 1 | Register client | Both ways |
| 2 | Patch control | Go → client |
| 3 | Control event | Client → Go |
| 4 | Update control props | Client → Go |
| 5 | Invoke method | Both ways |
| 6 | Session crashed | Go → client |

- **Method calls:** Go sends `{control_id, call_id, name, args}` with action 5. The client answers with the same action and `{control_id, call_id, result, error}`.
- **Page children:** besides `views`, the page holds `window` (id 2), `_overlay`, `_dialogs` and `_services` (a `ServiceRegistry` whose `_services` list holds Clipboard, FilePicker...).

- **Control:** a map with `_c` (type, e.g. `"Text"`), `_i` (id) and the properties. Events appear as `on_click: true`.
- **Reserved IDs:** 1 = Page, 2 = Window. Go controls start at 3.
- **Registration:** the client sends size, platform and theme. Go replies with the full page (`page_patch`).
- **Patch:** `[[tree], operation, …]`. Replace operation: `[0, node, property, value]`. There are also add (1), remove (2) and move (3).

### Current layout

```
app.go              → desktop: launches the client and accepts its connections
web.go              → web: serves the web client and the WebSocket, base path
uploads.go          → web: signed upload URLs and the upload endpoint
page.go             → tree, diff, dialogs and sending changes
router.go           → routes, groups, layouts, middleware, stacked views
navigation.go       → Page.Go, Page.Run and page events
state.go            → typed values per session (SessionKey)
session.go          → client messages, sessions and reconnection
invoke.go           → client method calls and the event queue
methods.go          → client methods written by hand
control.go          → control base and IDs
controls.go         → internal page root, overlay, dialogs and services
values.go           → Color, IconData, pointer helpers, value encoding
event.go            → base event
*_gen.go            → generated controls (including View), page settings,
                      value types, events, enums and methods
colors/, icons/     → generated named colors and icons
internal/protocol/  → message format
internal/client/    → finds, downloads and launches the client (desktop and web)
spec/               → Flet spec extracted from the Python SDK (+ icon tables)
tools/specgen/      → extractor (Python, standard library only)
tools/codegen/      → Go code generator
examples/counter/   → the counter
examples/list/      → add, remove and reorder list items
examples/gallery/   → sample of generated controls and inputs
examples/services/  → dialogs, snack bar, focus, clipboard, file picker, window
examples/web/       → the counter served to the browser
examples/upload/    → file uploads, served under /app/
examples/router/    → pages in separate files, layouts, stacked views, login
```

`page_test.go` includes a fake client that applies patches the same way the Dart client does, so the diff can be tested without opening a window.

The session lives in the root package, not in `internal/session`, to avoid an import cycle.

### How to run

Run the `examples/counter` example. On the first run, Flet client v1.1.2 is downloaded and cached (on macOS: `~/Library/Caches/flet-go/client/v1.1.2`). Later runs open it directly.

Optional environment variables:

| Variable | Purpose |
|---|---|
| `FLET_CLIENT_PATH` | Use an installed client (folder, `.app` or executable) |
| `FLET_CLIENT_URL` | Download the client from another URL (mirror or test) |
| `FLETGO_DEBUG=1` | Log the messages exchanged with the client |
| `FLET_WEB_PATH` | Use an extracted web client folder |
| `FLET_WEB_URL` | Download the web client from another URL |
| `FLET_ASSETS_DIR` | Folder with the app's own files (images, fonts, icons) |
| `FLET_SESSION_TIMEOUT` | Seconds a web session is kept after its tab disconnects (default 3600) |
| `FLET_MAX_UPLOAD_SIZE` | Maximum size of each uploaded file, in bytes |
| `FLET_SECRET_KEY` | Key that signs upload URLs |

Automatic download: macOS and Windows. On Linux, use `FLET_CLIENT_PATH` for now. The web client downloads on any OS.

### Web (phase 4)

Replace `flet.App(main)` with `flet.Serve(":8550", main)` and open `http://localhost:8550`. To mount it in your own server, use `flet.Handler(main)`, which returns an `http.Handler`.

Options go in `flet.WebOptions` (all optional): `BasePath`, `UploadDir`, `MaxUploadSize`, `SecretKey`.

- **Web client:** `flet-web.tar.gz` from the Flet release, downloaded and cached on the first run (on macOS: `~/Library/Caches/flet-go/web/v1.1.2`).
- **Transport:** a WebSocket at `/ws` (library: `github.com/coder/websocket`). Each binary message is one packet, `[type:u8][body]`, with no length prefix.
- **Files:** existing files are served as is; paths without an extension (app routes) get `index.html`; missing assets are a 404. Folders are not listed.
- **Sessions:** each browser tab is a session with its own page. A reloaded tab gets its page back. A disconnected session is dropped after `FLET_SESSION_TIMEOUT`, and `page.OnClose` runs.
- **Page info:** `page.Web`, `page.ClientIP` and `page.ClientUserAgent`; `page.OnConnect` and `page.OnDisconnect` follow the connection.
- **Base path:** with `BasePath: "/app/"` the app is served under `/app/` (mount it with `mux.Handle("/app/", h)`). The handler rewrites `index.html` so assets and the WebSocket use that path; `/app` redirects to `/app/`.
- **Uploads:** with `UploadDir` set, `page.GetUploadURL(name, expires)` returns a URL for `FilePicker.Upload`. The URL is signed with HMAC-SHA256 and works for that file name until it expires. Files are written with `os.Root`, so they cannot leave the upload folder (even through symbolic links), and only appear once complete. Progress arrives in `FilePicker.OnUpload`.
- **Security:** the WebSocket only accepts pages from the same host (default origin check).

Checked in the browser: the counter works, a reload keeps the count, a second tab has its own count, and there are no console errors.

### Code generation (phase 2)

The controls are generated from the Flet Python SDK, which is the most complete description of the client's controls:

1. `python3 -I tools/specgen/extract.py <flet-checkout> spec/flet.json` reads the Python sources with `ast` and writes the spec. Only needed when updating the Flet version.
2. `go generate ./...` runs `tools/codegen`, which writes `*_gen.go`, `colors/` and `icons/`.

Generated for v1.1.2: 152 controls (including services and the window), 134 value types, 63 typed events, 96 enums, 111 client methods, ~400 colors and ~10k icons.

Type mapping rules:

| Python | Go |
|---|---|
| `str`, `bool`, `int`, `Number` | `string`, `bool`, `int`, `float64` (zero = not sent) |
| Same, with a non-zero default (e.g. `visible=True`, `opacity=1.0`) | Pointer: `*bool`, `*float64` (use `flet.Bool`, `flet.Float`) |
| Enum | Named type with constants, e.g. `flet.FontWeightBold` |
| `ColorValue`, `IconData` | `flet.Color` (`colors` package), `flet.IconData` (`icons` package) |
| `DurationValue` | `time.Duration` (sent in milliseconds) |
| Value dataclass | Pointer to a generated struct, e.g. `*flet.TextStyle` |
| `Control`, `list[Control]` | `Control`, `[]Control` (tracked as children) |
| `str` or `Control` | `any` holding a string or a control |
| Other unions (e.g. `PaddingValue`) | `any`, e.g. `Padding: 16` or `Padding: &flet.Padding{...}` |
| `on_*` handlers | `func(e *flet.Event)` or a typed event, e.g. `func(e *flet.TapEvent)` |

Properties changed on the client (e.g. `TextField.value` while typing) are applied back to the Go struct.

Value types follow the Python encoding: `@value` types send only what differs from the default, while plain `@dataclass` types (e.g. `FilePickerUploadFile`) always send their defaults, because the client expects them (e.g. `method: "PUT"`).

Client methods (e.g. `focus()`, `Clipboard.get()`) become blocking Go methods that return an error, plus the result when there is one:

| Python | Go |
|---|---|
| `await field.focus()` | `err := field.Focus()` |
| `await clipboard.get()` | `text, err := clipboard.Get()` |
| `await picker.pick_files(allow_multiple=True)` | `files, err := picker.PickFiles(flet.PickFilesOptions{AllowMultiple: true})` |

Required parameters are positional; optional ones go in an `...Options` struct. Methods that do more than one client call are written by hand in `methods.go`.

### Methods, dialogs and services (phase 3)

- **Event queue:** handlers run one at a time on a goroutine per page, so a handler can call a client method and wait while the session keeps reading.
- **Dialogs:** `page.ShowDialog(d)` and `page.PopDialog()`. A dialog leaves the page when the client reports it was dismissed.
- **Services:** add them to `page.Services` before calling their methods.
- **Window:** `page.Window` has the window properties and methods (`Center`, `Close`, `ToFront`...). Its size is filled in from the client.
- **Page info:** `page.Width`, `Height`, `Platform`, `PlatformBrightness`, `Web` and `Route` come from the client.
- **Reconnection:** if the connection drops while the window is open, the client reconnects with its session id and gets the same page back; `main` does not run again. Method calls that were waiting fail with `ErrDisconnected`.
- **Errors:** a panic in `main` or in a handler is logged and shown in the client.

### Pages, routing and assets

**Page settings and views.** `Page` embeds the generated `PageProps`: `Theme`, `DarkTheme`, `ThemeMode`, `Fonts`, `LocaleConfiguration` and the page events (`OnResize`, `OnKeyboardEvent`, `OnRouteChange`, `OnViewPop`, `OnPlatformBrightnessChange`, `OnAppLifecycleStateChange`...). `page.Views` is the stack of generated `View` controls, each with `AppBar`, `NavigationBar`, `Drawer`, `FloatingActionButton`, `Padding`, `Bgcolor`, `Scroll`... `page.Add` works on the first view.

**Router.** Each page is a function, usually in its own file, connected to a route in `main.go`:

```go
r := flet.NewRouter()
r.Layout(pages.MainLayout)                  // default layout
r.Route("/", pages.Home)
r.Route("/login", pages.Login, flet.NoLayout())
products := r.Group("/products", nil, flet.Stacked())
products.Route("/", pages.Products)
products.Route("/{id}", pages.Product)      // ctx.Param("id")
admin := r.Group("/admin", pages.AdminLayout)
admin.Use(pages.RequireLogin)               // middleware: ctx.Redirect("/login")
admin.Route("/", pages.Dashboard)
r.NotFound(pages.NotFound)
flet.Serve(":8550", r.Main)                 // or flet.App(r.Main)
```

- A page is `func(ctx *flet.RouteContext) flet.Control`. It runs every time its route opens. Returning a `*flet.View` gives it the whole screen and skips the layout.
- Patterns follow `net/http`: `{name}` is one segment, `{name...}` the rest. A fixed segment beats a parameter, which beats a wildcard.
- Layouts: the route's own (`WithLayout`, `NoLayout`), then the nearest group's, then the router's.
- `ctx.Go(route)` (or `page.Go`) shows the route and updates the browser address. The browser's back and forward buttons, typed addresses, deep links and reloads all go through the router.
- `Stacked()` builds the stack from the path: `/products/42` shows `/products` and `/` underneath, with a back arrow. Screens already on the stack are kept, not rebuilt.
- `flet.NewSessionKey` stores typed values per session (logged-in user, cart), shared by every page.

**Updates from other goroutines.** `page.Run(fn)` runs `fn` on the page's event queue, so it never overlaps a handler. Use it for timers, chat messages and background jobs.

**Assets.** `WebOptions.AssetsDir` and `AppOptions.AssetsDir` (or `FLET_ASSETS_DIR`) point to a folder with the app's files: `assets/logo.png` is used as `&flet.Image{Src: "logo.png"}`. On the web its files take precedence over the client's (to replace `favicon.png`, `icons/`, `manifest.json` or `index.html`). On desktop the folder is passed to the client.

Checked in the browser with `examples/router`: layouts, the navigation bar, the app's logo, stacked screens with back arrows, a deep link to `/products/2`, the login redirect and the browser's back button.

### Known limitations

- Events are handled one at a time, in arrival order. A handler waiting on a slow method (e.g. the file picker) holds back the next events.
- Desktop sessions are kept while the window is open; web sessions expire (see "Web").
- Without `FLET_SECRET_KEY` (or `SecretKey`), upload URLs stop working when the server restarts.
- Page methods `get_device_info`, `get_package_info` and `set_allowed_device_orientations` are not exposed yet.
- `page.Go` and `page.Update` from another goroutine must go through `page.Run`.
- `Share` methods are not generated yet.
- Automatic download not yet tested on Windows and not supported on Linux.
- Not generated yet: canvas shapes, extension packages (charts, maps, video...), and 24 fields with rare types (dates, times).

---

## 11. Next steps

1. ~~Choose between option A and B~~ → option A.
2. ~~Study the protocol~~ → section 10.
3. ~~Control tree, IDs, diff and event dispatch~~.
4. ~~Counter working end to end~~.
5. ~~Automatic client download~~ → macOS and Windows.
6. ~~Item-by-item list diff and index cleanup~~ → add/remove/move ops, tested against a simulated client and the real one (`examples/list`).
7. ~~Phase 2: control generator from Flet's definitions~~ → see "Code generation".
8. ~~Phase 3: control methods, dialogs, services, reconnection~~ → see "Methods, dialogs and services".
9. ~~Phase 4: server-backed web~~ → see "Web".
10. ~~Web refinements: page settings, views, router, Page.Run, assets~~ → see "Pages, routing and assets".
11. Web production: compression, cache headers, app name and icons, Flutter options (renderer, no CDN), graceful shutdown, and `flet-go build web` (one binary with the client embedded).
12. Phase 5–6: Android and iOS (Go as a library started by the Flutter app).
