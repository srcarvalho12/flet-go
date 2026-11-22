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
| 3. Full diff, methods, reconnection | Efficient updates and multiple users | 3 to 5 days |
| 4. Server-backed web | Go binary serving the app | 1 to 2 days |
| 5–6. Android + iOS | App running on device | 1 to 3 weeks |
| 7. CLI and packaging | Something like `flet-go build` | 2 to 4 days |

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

- **Control:** a map with `_c` (type, e.g. `"Text"`), `_i` (id) and the properties. Events appear as `on_click: true`.
- **Reserved IDs:** 1 = Page, 2 = Window. Go controls start at 3.
- **Registration:** the client sends size, platform and theme. Go replies with the full page (`page_patch`).
- **Patch:** `[[tree], operation, …]`. Replace operation: `[0, node, property, value]`. There are also add (1), remove (2) and move (3).

### Current layout

```
app.go              → launches the client and waits for the connection
page.go             → tree, diff and sending changes
session.go          → client messages
control.go          → control base and IDs
controls.go         → internal page root and view
values.go           → Color, IconData, pointer helpers, value encoding
event.go            → base event
*_gen.go            → generated controls, value types, events and enums
colors/, icons/     → generated named colors and icons
internal/protocol/  → message format
internal/client/    → finds, downloads and launches the client
spec/               → Flet spec extracted from the Python SDK (+ icon tables)
tools/specgen/      → extractor (Python, standard library only)
tools/codegen/      → Go code generator
examples/counter/   → the counter
examples/list/      → add, remove and reorder list items
examples/gallery/   → sample of generated controls and inputs
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

Automatic download: macOS and Windows. On Linux, use `FLET_CLIENT_PATH` for now.

### Code generation (phase 2)

The controls are generated from the Flet Python SDK, which is the most complete description of the client's controls:

1. `python3 -I tools/specgen/extract.py <flet-checkout> spec/flet.json` reads the Python sources with `ast` and writes the spec. Only needed when updating the Flet version.
2. `go generate ./...` runs `tools/codegen`, which writes `*_gen.go`, `colors/` and `icons/`.

Generated for v1.1.2: 132 controls, 134 value types, 63 typed events, 96 enums, ~400 colors and ~10k icons.

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

### Known limitations

- Events are handled one at a time, in arrival order.
- No reconnection.
- Automatic download not yet tested on Windows and not supported on Linux.
- Not generated yet: canvas shapes, services (FilePicker, Clipboard...), extension packages (charts, maps, video...), control methods (e.g. `focus()`, dialogs), and 28 fields with rare types (dates, bytes).

---

## 11. Next steps

1. ~~Choose between option A and B~~ → option A.
2. ~~Study the protocol~~ → section 10.
3. ~~Control tree, IDs, diff and event dispatch~~.
4. ~~Counter working end to end~~.
5. ~~Automatic client download~~ → macOS and Windows.
6. ~~Item-by-item list diff and index cleanup~~ → add/remove/move ops, tested against a simulated client and the real one (`examples/list`).
7. ~~Phase 2: control generator from Flet's definitions~~ → see "Code generation".
8. Phase 3: control methods (invoke method), dialogs and services, reconnection.
