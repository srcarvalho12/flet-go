package flet

import (
	"context"
	"errors"
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

const testIndex = `<html><head><base href="/"><!-- fletAppConfig --></head>index</html>`

// newWebServer serves main with a fake web client folder.
func newWebServer(t *testing.T, main func(*Page), opts ...WebOptions) (*httptest.Server, *webHandler) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte(testIndex), 0o644)
	os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("js"), 0o644)
	t.Setenv(client.EnvWebPath, dir)

	h, err := Handler(main, opts...)
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
		{"/", testIndex, 200},
		{"/index.html", testIndex, 200},
		{"/assets/app.js", "js", 200},
		{"/some/route", testIndex, 200}, // app route
		{"/missing.js", "", 404},
		{"/assets", testIndex, 200}, // folders are not listed
		{"/upload", testIndex, 200}, // no upload endpoint unless enabled
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
	return dialWebPath(t, ts, "/ws", userAgent...)
}

func dialWebPath(t *testing.T, ts *httptest.Server, wsPath string, userAgent ...string) *wsClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	opts := &websocket.DialOptions{HTTPHeader: http.Header{}}
	if len(userAgent) > 0 {
		opts.HTTPHeader.Set("User-Agent", userAgent[0])
	}
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+wsPath, opts)
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

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestWebBasePath(t *testing.T) {
	ts, _ := newWebServer(t, func(p *Page) { p.Add(&Text{Value: "hi"}) }, WebOptions{BasePath: "app"})

	code, body := get(t, ts.URL+"/app/")
	if code != 200 || !strings.Contains(body, `<base href="/app/">`) ||
		!strings.Contains(body, `flet.webSocketEndpoint="/app/ws"`) ||
		!strings.Contains(body, `flet.assetBase="/app/"`) {
		t.Fatalf("index not patched: %d %s", code, body)
	}
	if code, body := get(t, ts.URL+"/app/assets/app.js"); code != 200 || body != "js" {
		t.Errorf("asset: %d %q", code, body)
	}
	if code, _ := get(t, ts.URL+"/app"); code != http.StatusMovedPermanently {
		t.Errorf("/app: %d, want redirect", code)
	}
	for _, p := range []string{"/", "/assets/app.js", "/application", "/ws"} {
		if code, _ := get(t, ts.URL+p); code != 404 {
			t.Errorf("%s outside the base path: %d, want 404", p, code)
		}
	}

	c := dialWebPath(t, ts, "/app/ws")
	if c.register("") == "" {
		t.Fatal("no session over the base path WebSocket")
	}
}

func TestUploads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvSecretKey, "")
	t.Setenv(EnvMaxUploadSize, "")
	urls := make(chan *Page, 1)
	ts, h := newWebServer(t, func(p *Page) { urls <- p }, WebOptions{
		BasePath: "/app/", UploadDir: dir, MaxUploadSize: 10,
	})
	dialWebPath(t, ts, "/app/ws").register("")
	page := <-urls

	put := func(u, body string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPut, ts.URL+u, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	u, err := page.GetUploadURL("docs/a.txt", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "/app/upload?") {
		t.Fatalf("upload URL %q is not under the base path", u)
	}
	if code := put(u, "hello"); code != 200 {
		t.Fatalf("upload: %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "docs", "a.txt")); string(b) != "hello" {
		t.Fatalf("saved %q", b)
	}

	if code := put(strings.Replace(u, "a.txt", "b.txt", 1), "x"); code != http.StatusForbidden {
		t.Errorf("URL for another file: %d, want 403", code)
	}
	if code := put(u, "more than ten bytes"); code != http.StatusRequestEntityTooLarge {
		t.Errorf("too large: %d, want 413", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "a.txt")); err != nil {
		t.Errorf("a rejected upload removed the earlier file: %v", err)
	}

	expired, _ := h.srv.uploads.url("old.txt", -time.Second)
	if code := put(expired, "x"); code != http.StatusGone {
		t.Errorf("expired: %d, want 410", code)
	}
	for _, name := range []string{"../escape.txt", "/etc/x", "", "a/../../b"} {
		if _, err := page.GetUploadURL(name, time.Minute); err == nil {
			t.Errorf("GetUploadURL(%q) should fail", name)
		}
	}

	// Leftover temporary files would mean a failed upload was not cleaned up.
	entries, _ := os.ReadDir(filepath.Join(dir, "docs"))
	if len(entries) != 1 {
		t.Errorf("upload folder has %d entries, want 1", len(entries))
	}

	if _, err := newTestPageNoT().GetUploadURL("a", time.Minute); !errors.Is(err, ErrUploadsDisabled) {
		t.Errorf("err = %v, want ErrUploadsDisabled", err)
	}
}

func newTestPageNoT() *Page { return newPage(func(protocol.Message) {}) }
