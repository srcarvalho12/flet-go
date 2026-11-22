package flet

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/srcarvalho12/flet-go/internal/client"
	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// EnvSessionTimeout sets, in seconds, how long a web session is kept after
// its browser tab disconnects (default 3600, like Flet).
const EnvSessionTimeout = "FLET_SESSION_TIMEOUT"

const defaultSessionTimeout = time.Hour

// maxMessageSize limits a single message from the browser (e.g. picked files
// read with their data).
const maxMessageSize = 64 << 20

// Serve runs the app as a web server at addr (e.g. ":8550").
// Each browser tab gets its own page. The web client is downloaded on the
// first run and served from the same address.
func Serve(addr string, main func(page *Page)) error {
	h, err := Handler(main)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Serving on http://%s\n", displayAddr(ln.Addr()))
	return http.Serve(ln, h)
}

// Handler returns an http.Handler that serves the web client and runs main
// for each browser tab. It must be mounted at the root path ("/").
func Handler(main func(page *Page)) (http.Handler, error) {
	dir, err := client.ResolveWeb()
	if err != nil {
		return nil, err
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return nil, fmt.Errorf("flet: web client: %w", err)
	}

	srv := newServer(main, os.Getenv(EnvDebug) != "")
	srv.timeout = defaultSessionTimeout
	if v := os.Getenv(EnvSessionTimeout); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs < 0 {
			return nil, fmt.Errorf("flet: invalid %s: %q", EnvSessionTimeout, v)
		}
		srv.timeout = time.Duration(secs) * time.Second
	}
	return &webHandler{srv: srv, dir: dir, index: index}, nil
}

type webHandler struct {
	srv   *server
	dir   string
	index []byte
}

func (h *webHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := path.Clean("/" + r.URL.Path)
	switch {
	case p == "/ws":
		h.serveWebSocket(w, r)
		return
	case p == "/" || p == "/index.html":
		h.serveIndex(w)
		return
	}
	file := filepath.Join(h.dir, filepath.FromSlash(p))
	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		http.ServeFile(w, r, file)
		return
	}
	// App routes such as /settings load the app; missing assets are a 404.
	if path.Ext(p) == "" {
		h.serveIndex(w)
		return
	}
	http.NotFound(w, r)
}

func (h *webHandler) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(h.index)
}

func (h *webHandler) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	// The default origin check rejects pages from other sites.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	c.SetReadLimit(maxMessageSize)
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	t := &wsTransport{c: c, ctx: r.Context()}
	err = h.srv.serve(t, clientInfo{ip: ip, userAgent: r.UserAgent()})
	if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure ||
		status == websocket.StatusGoingAway || errors.Is(err, context.Canceled) {
		err = nil
	}
	// Tabs closing abruptly are normal on the web; only log them when debugging.
	if err != nil && h.srv.debug {
		log.Printf("flet: web connection: %v", err)
	}
	c.CloseNow()
}

// wsTransport is the web transport: one packet per binary WebSocket message.
type wsTransport struct {
	c   *websocket.Conn
	ctx context.Context
}

func (t *wsTransport) Read() (protocol.Message, error) {
	for {
		typ, data, err := t.c.Read(t.ctx)
		if err != nil {
			return protocol.Message{}, err
		}
		if typ != websocket.MessageBinary {
			continue
		}
		m, ok, err := protocol.Decode(data)
		if err != nil {
			return protocol.Message{}, err
		}
		if ok {
			return m, nil
		}
	}
}

func (t *wsTransport) Write(m protocol.Message) error {
	data, err := protocol.Encode(m)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(t.ctx, 30*time.Second)
	defer cancel()
	return t.c.Write(ctx, websocket.MessageBinary, data)
}

func displayAddr(a net.Addr) string {
	host, port, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	if ip := net.ParseIP(host); host == "" || ip != nil && ip.IsUnspecified() {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}
