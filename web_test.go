package flet

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/srcarvalho12/flet-go/internal/client"
	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// newWebServer serves main with a fake web client folder.
func newWebServer(t *testing.T, main func(*Page)) (*httptest.Server, *webHandler) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>index</html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("js"), 0o644)
	t.Setenv(client.EnvWebPath, dir)

	h, err := Handler(main)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, h.(*webHandler)
}

func TestWebStaticFiles(t *testing.T) {
	ts, _ := newWebServer(t, func(*Page) {})
	cases := []struct {
		path, body string
		status     int
	}{
		{"/", "<html>index</html>", 200},
		{"/index.html", "<html>index</html>", 200},
		{"/assets/app.js", "js", 200},
		{"/some/route", "<html>index</html>", 200}, // app route
		{"/missing.js", "", 404},
		{"/assets", "<html>index</html>", 200}, // folders are not listed
	}
	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.status || c.body != "" && string(b) != c.body {
			t.Errorf("%s: %d %q, want %d %q", c.path, resp.StatusCode, b, c.status, c.body)
		}
	}
}

type wsClient struct {
	t *testing.T
	c *websocket.Conn
}

func dialWeb(t *testing.T, ts *httptest.Server, userAgent ...string) *wsClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	opts := &websocket.DialOptions{HTTPHeader: http.Header{}}
	if len(userAgent) > 0 {
		opts.HTTPHeader.Set("User-Agent", userAgent[0])
	}
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", opts)
	if err != nil {
		t.Fatal(err)
	}
	c.SetReadLimit(maxMessageSize)
	t.Cleanup(func() { c.CloseNow() })
	return &wsClient{t: t, c: c}
}

func (w *wsClient) send(action int, body map[string]any) {
	w.t.Helper()
	data, _ := protocol.Encode(protocol.Message{Action: action, Body: body})
	if err := w.c.Write(context.Background(), websocket.MessageBinary, data); err != nil {
		w.t.Fatal(err)
	}
}

func (w *wsClient) next(action int) protocol.Message {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		_, data, err := w.c.Read(ctx)
		if err != nil {
			w.t.Fatalf("waiting for action %d: %v", action, err)
		}
		m, ok, err := protocol.Decode(data)
		if err != nil {
			w.t.Fatal(err)
		}
		if ok && m.Action == action {
			return m
		}
	}
}

func (w *wsClient) register(sid string) string {
	w.t.Helper()
	w.send(protocol.ActionRegisterClient, map[string]any{
		"session_id": sid,
		"page":       map[string]any{"web": true, "platform": "macos"},
	})
	return w.next(protocol.ActionRegisterClient).Body["session_id"].(string)
}

// Each tab gets its own page; events work over the WebSocket.
func TestWebSessions(t *testing.T) {
	type tab struct {
		btn *Button
		txt *Text
	}
	tabs := make(chan tab, 2)
	var mu sync.Mutex
	byAgent := map[string]tab{}
	ts, _ := newWebServer(t, func(p *Page) {
		txt := &Text{Value: "0"}
		btn := &Button{Content: "+", OnClick: func(*Event) { txt.Value = "1"; p.Update() }}
		p.Add(btn, txt)
		if !p.Web || p.ClientIP == "" {
			t.Errorf("web page info missing: web=%v ip=%q", p.Web, p.ClientIP)
		}
		mu.Lock()
		byAgent[p.ClientUserAgent] = tab{btn, txt}
		mu.Unlock()
		tabs <- tab{btn, txt}
	})

	a, b := dialWeb(t, ts, "tab-a"), dialWeb(t, ts, "tab-b")
	if a.register("") == b.register("") {
		t.Fatal("two tabs share a session")
	}
	<-tabs
	<-tabs
	mu.Lock()
	ta, tb := byAgent["tab-a"], byAgent["tab-b"]
	mu.Unlock()
	if ta.btn == nil || tb.btn == nil || ta.btn.ID() == tb.btn.ID() {
		t.Fatal("tabs do not have their own controls")
	}

	a.next(protocol.ActionPatchControl) // controls added by main
	a.send(protocol.ActionControlEvent, map[string]any{"target": ta.btn.ID(), "name": "click"})
	if patch := a.next(protocol.ActionPatchControl); patch.Body["id"] != ta.txt.ID() {
		t.Fatalf("unexpected patch %v", patch.Body)
	}
	if tb.txt.Value != "0" {
		t.Fatal("a click in one tab changed the other")
	}
}

// A tab that comes back within the timeout keeps its page; after it, the
// session is gone and OnClose runs.
func TestWebSessionExpiry(t *testing.T) {
	t.Setenv(EnvSessionTimeout, "1")
	closed := make(chan struct{})
	var once sync.Once
	ts, h := newWebServer(t, func(p *Page) {
		// Only the first page: the last one expires after the test ends.
		once.Do(func() { p.OnClose = func(*Event) { close(closed) } })
	})

	a := dialWeb(t, ts)
	sid := a.register("")
	a.c.Close(websocket.StatusNormalClosure, "")

	b := dialWeb(t, ts)
	if got := b.register(sid); got != sid {
		t.Fatalf("reconnect within timeout got a new session")
	}
	b.c.Close(websocket.StatusNormalClosure, "")

	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("session did not expire")
	}
	h.srv.mu.Lock()
	n := len(h.srv.pages)
	h.srv.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d sessions left after expiry", n)
	}

	c := dialWeb(t, ts)
	if got := c.register(sid); got == sid {
		t.Fatal("expired session was reused")
	}
}
