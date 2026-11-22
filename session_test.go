package flet

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// pipeClient is the client end of an in-memory connection to a server.
type pipeClient struct {
	t    *testing.T
	conn net.Conn
	in   chan protocol.Message
}

func dial(t *testing.T, srv *server) *pipeClient {
	t.Helper()
	cc, sc := net.Pipe()
	go srv.serve(socketTransport{sc}, clientInfo{})
	c := &pipeClient{t: t, conn: cc, in: make(chan protocol.Message, 100)}
	go func() {
		defer close(c.in)
		for {
			m, err := protocol.Read(cc)
			if err != nil {
				return
			}
			c.in <- m
		}
	}()
	t.Cleanup(func() { cc.Close() })
	return c
}

func (c *pipeClient) send(action int, body map[string]any) {
	c.t.Helper()
	if err := protocol.Write(c.conn, protocol.Message{Action: action, Body: body}); err != nil {
		c.t.Fatal(err)
	}
}

// next waits for the next message with the given action, skipping others.
func (c *pipeClient) next(action int) protocol.Message {
	c.t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case m, ok := <-c.in:
			if !ok {
				c.t.Fatalf("connection closed while waiting for action %d", action)
			}
			if m.Action == action {
				return m
			}
		case <-timeout:
			c.t.Fatalf("timeout waiting for action %d", action)
		}
	}
}

func (c *pipeClient) register(sessionID string) protocol.Message {
	c.t.Helper()
	c.send(protocol.ActionRegisterClient, map[string]any{
		"session_id": sessionID,
		"page":       map[string]any{"width": 800, "height": 600, "platform": "macos"},
	})
	return c.next(protocol.ActionRegisterClient)
}

// A handler waits for a client method result while the session keeps reading.
func TestMethodCallFromHandler(t *testing.T) {
	clip := &Clipboard{}
	result := &Text{}
	btn := &Button{Content: "paste"}
	var page *Page
	started := make(chan struct{})
	srv := newServer(func(p *Page) {
		page = p
		p.Services = append(p.Services, clip)
		btn.OnClick = func(e *Event) {
			v, err := clip.Get()
			if err != nil {
				result.Value = "error: " + err.Error()
			} else {
				result.Value = v
			}
			e.Page.Update()
		}
		p.Add(btn, result)
		close(started)
	}, false)

	c := dial(t, srv)
	reg := c.register("")
	if reg.Body["session_id"] == "" {
		t.Fatal("no session id")
	}
	<-started
	if page.Width != 800 || page.Platform != "macos" {
		t.Errorf("page info not applied: %v %v", page.Width, page.Platform)
	}

	c.send(protocol.ActionControlEvent, map[string]any{"target": btn.ID(), "name": "click"})
	call := c.next(protocol.ActionInvokeMethod)
	if call.Body["name"] != "get" || call.Body["control_id"] != clip.ID() {
		t.Fatalf("unexpected call %v", call.Body)
	}
	c.send(protocol.ActionInvokeMethod, map[string]any{
		"control_id": clip.ID(), "call_id": call.Body["call_id"], "result": "hello", "error": nil,
	})
	patch := c.next(protocol.ActionPatchControl)
	if patch.Body["id"] != result.ID() {
		t.Fatalf("patch for %v, want %d", patch.Body["id"], result.ID())
	}

	// An error from the client becomes a MethodError.
	c.send(protocol.ActionControlEvent, map[string]any{"target": btn.ID(), "name": "click"})
	call = c.next(protocol.ActionInvokeMethod)
	c.send(protocol.ActionInvokeMethod, map[string]any{
		"control_id": clip.ID(), "call_id": call.Body["call_id"], "result": nil, "error": "boom",
	})
	c.next(protocol.ActionPatchControl)
	if want := "error: flet: Clipboard.get: boom"; result.Value != want {
		t.Errorf("result = %q, want %q", result.Value, want)
	}
}

func TestMethodOnUnmountedControl(t *testing.T) {
	if _, err := (&Clipboard{}).Get(); !errors.Is(err, ErrNotMounted) {
		t.Fatalf("err = %v, want ErrNotMounted", err)
	}
}

// After a reconnect with the same session id, the page is sent again and
// main does not run twice. Calls waiting during the drop fail.
func TestReconnect(t *testing.T) {
	runs := 0
	clip := &Clipboard{}
	txt := &Text{Value: "kept"}
	callErr := make(chan error, 1)
	srv := newServer(func(p *Page) {
		runs++
		p.Services = []Control{clip}
		p.Add(txt)
		go func() {
			_, err := clip.Get()
			callErr <- err
		}()
	}, false)

	c := dial(t, srv)
	sid := c.register("").Body["session_id"].(string)
	c.next(protocol.ActionInvokeMethod)
	c.conn.Close()

	select {
	case err := <-callErr:
		if !errors.Is(err, ErrDisconnected) {
			t.Fatalf("err = %v, want ErrDisconnected", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending call did not fail on disconnect")
	}

	c2 := dial(t, srv)
	reg := c2.register(sid)
	if reg.Body["session_id"] != sid {
		t.Fatalf("session id changed: %v", reg.Body["session_id"])
	}
	fc := &fakeClient{t: t, root: toClient(reg.Body["page_patch"]).(map[string]any)}
	if fc.find(fc.root, txt.ID()) == nil {
		t.Fatal("reconnected page lost its controls")
	}
	if runs != 1 {
		t.Fatalf("main ran %d times", runs)
	}

	// Updates keep working on the new connection.
	txt.Value = "changed"
	srv.pages[sid].page.Update()
	if p := c2.next(protocol.ActionPatchControl); p.Body["id"] != txt.ID() {
		t.Fatalf("patch for %v", p.Body["id"])
	}
}

func TestDialogs(t *testing.T) {
	p, fc := newTestPage(t)
	connect(p, fc)

	dismissed := 0
	dlg := &AlertDialog{Title: "Hello", OnDismiss: func(*Event) { dismissed++ }}
	if err := p.ShowDialog(dlg); err != nil {
		t.Fatal(err)
	}
	assertInSync(t, p, fc)
	if !dlg.Open {
		t.Fatal("dialog not open")
	}
	if err := p.ShowDialog(dlg); err == nil {
		t.Fatal("showing an open dialog twice should fail")
	}
	if err := p.ShowDialog(&Text{}); err == nil {
		t.Fatal("a Text is not a dialog")
	}

	if p.PopDialog() != dlg || dlg.Open {
		t.Fatal("PopDialog did not close the dialog")
	}
	assertInSync(t, p, fc)

	// The client reports the dismissal: the dialog leaves the page.
	p.dispatch(dlg.ID(), "dismiss", nil)
	assertInSync(t, p, fc)
	if dismissed != 1 {
		t.Fatalf("OnDismiss called %d times", dismissed)
	}
	if p.lookup(dlg.ID()) != nil {
		t.Fatal("dismissed dialog is still in the index")
	}
	if p.PopDialog() != nil {
		t.Fatal("no dialog should be open")
	}
}

func TestOverlayAndWindow(t *testing.T) {
	p, fc := newTestPage(t)
	connect(p, fc)

	p.Overlay = append(p.Overlay, &Text{Value: "floating"})
	p.Window.Width = 400
	p.Update()
	assertInSync(t, p, fc)
	if p.Window.ID() != 2 {
		t.Fatalf("window id = %d", p.Window.ID())
	}

	p.applyClientProps(2, map[string]any{"width": 500.0})
	if p.Window.Width != 500 {
		t.Fatalf("window width = %v", p.Window.Width)
	}
}

// A tab reloaded at another address reconnects to its session and shows
// that address.
func TestReconnectAtAnotherRoute(t *testing.T) {
	r := NewRouter()
	r.Route("/", func(*RouteContext) Control { return &Text{Value: "home"} })
	r.Route("/other", func(*RouteContext) Control { return &Text{Value: "other"} })
	srv := newServer(r.Main, false)

	c := dial(t, srv)
	sid := c.register("").Body["session_id"].(string)
	c.next(protocol.ActionPatchControl)
	c.conn.Close()

	c2 := dial(t, srv)
	c2.send(protocol.ActionRegisterClient, map[string]any{
		"session_id": sid,
		"page":       map[string]any{"route": "/other"},
	})
	c2.next(protocol.ActionRegisterClient)
	c2.next(protocol.ActionPatchControl)
	srv.mu.Lock()
	p := srv.pages[sid].page
	srv.mu.Unlock()
	done := make(chan string)
	p.Run(func() { done <- texts(p.Views[0]) })
	if got := <-done; got != "other" {
		t.Fatalf("reconnected tab shows %q, want other", got)
	}
}

// Optional method arguments that default to None can be sent as zero.
func TestMethodZeroArgument(t *testing.T) {
	p, fc := newTestPage(t)
	connect(p, fc)
	list := &ListView{}
	p.Add(list)
	if err := list.ScrollTo(ScrollToOptions{Offset: Float(0)}); err != nil {
		t.Fatal(err)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "scroll_to map[offset:0]" {
		t.Fatalf("calls = %v", fc.calls)
	}
}
