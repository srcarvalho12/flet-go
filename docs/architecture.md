# Architecture

flet-go is a Go SDK for the Flet client. The client is a Flutter app that renders a
tree of controls and reports user actions. All logic and state live in Go.

```
┌──────────────────────┐       messages       ┌──────────────────────┐
│       Your Go app    │  ─── add/update ───▶ │  Flet Flutter client │
│     control tree     │                      │   renders widgets    │
│   diff + handlers    │  ◀──── events ────── │     sends events     │
└──────────────────────┘  socket / WebSocket  └──────────────────────┘
```

## Session flow

1. Go starts a server: a local TCP socket on desktop, HTTP and a WebSocket on the
   web.
2. The client connects and registers (size, platform, route).
3. Go answers with the full page, then runs `main` on the page's event queue.
4. On each event, Go runs the handler; `page.Update()` sends only what changed.

## Protocol

flet-go speaks the Flet client protocol (pinned to client v1.1.2), encoded with
MessagePack (`github.com/vmihailenco/msgpack/v5`).

- **Desktop packet:** `[length:u32 LE][type:u8][body]`.
- **Web:** one WebSocket binary message per packet: `[type:u8][body]`.
- Type `0x00` is a message `[action, body]`.

| Action | Name | Direction |
|---|---|---|
| 1 | Register client | Both |
| 2 | Patch control | Go → client |
| 3 | Control event | Client → Go |
| 4 | Update control props | Client → Go |
| 5 | Invoke method | Both |
| 6 | Session crashed | Go → client |

Controls are maps with `_c` (type) and `_i` (id). Ids 1 and 2 are the page and the
window.

## Diffing

For each control, Go keeps a snapshot of the last state sent: its properties,
the ids in each child list, and its single children. `page.Update()` walks the tree
and sends, per control:

- replaced properties;
- list operations item by item: remove, move and add;
- whole replacement of single children that changed.

Removed controls leave the event index once the update is sent. Values changed by
the client (e.g. text typed in a field) are applied to the Go structs and marked
as sent, so they are not echoed back.

## Concurrency

- One goroutine reads each connection.
- Events, client property changes and `page.Run` functions go through a queue per
  page, run one at a time.
- Client method results are delivered by the reader directly, so a handler can
  wait for a result without blocking the session.

## Code generation

The controls are generated from the Flet Python SDK, the most complete description
of the client's controls:

1. `tools/specgen/extract.py` reads the Python sources with `ast` (standard
   library only) and writes `spec/flet.json`.
2. `tools/codegen` reads the spec and writes `controls_gen.go`, `page_gen.go`,
   `values_gen.go`, `events_gen.go`, `enums_gen.go`, `methods_gen.go`, `colors/`
   and `icons/`.

```bash
python3 -I tools/specgen/extract.py <flet-checkout> spec/flet.json   # only when updating Flet
go generate ./...
```

Type mapping, inheritance (C3 linearization), defaults and method extraction are
described in [PLAN.md](../PLAN.md#code-generation-phase-2).

## Layout

| Path | Contents |
|---|---|
| `app.go` | Desktop: launches the client |
| `web.go`, `uploads.go` | Web: client files, WebSocket, base path, uploads |
| `page.go` | Tree, diff, dialogs |
| `router.go`, `navigation.go`, `state.go` | Router, `Page.Go`, `Page.Run`, session values |
| `session.go`, `invoke.go` | Sessions, reconnection, method calls, event queue |
| `control.go`, `controls.go`, `values.go` | Control base, internal containers, value encoding |
| `*_gen.go`, `colors/`, `icons/` | Generated code |
| `internal/protocol` | Message format |
| `internal/client` | Finds, downloads and launches the client |
| `tools/` | Spec extractor and code generator |
