package flet

import (
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"sync"

	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// server keeps the pages of all sessions, so a client that reconnects with
// its session id gets its page back instead of a fresh one.
type server struct {
	main  func(*Page)
	debug bool

	mu    sync.Mutex
	pages map[string]*Page
}

func newServer(main func(*Page), debug bool) *server {
	return &server{main: main, debug: debug, pages: map[string]*Page{}}
}

// serve runs one client connection until it closes.
func (srv *server) serve(conn net.Conn) error {
	s := &session{srv: srv, conn: conn}
	err := s.run()
	if s.page != nil {
		s.page.disconnect()
	}
	return err
}

// session is one connection with a client.
type session struct {
	srv  *server
	conn net.Conn
	page *Page
	wmu  sync.Mutex
}

func (s *session) send(m protocol.Message) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.srv.debug {
		log.Printf("→ action %d: %v", m.Action, m.Body)
	}
	if err := protocol.Write(s.conn, m); err != nil {
		log.Printf("flet: send error: %v", err)
	}
}

// run reads messages until the connection closes.
func (s *session) run() error {
	for {
		m, err := protocol.Read(s.conn)
		if err != nil {
			return err
		}
		if s.srv.debug {
			log.Printf("← action %d: %v", m.Action, m.Body)
		}
		s.handle(m)
	}
}

func (s *session) handle(m protocol.Message) {
	if m.Action == protocol.ActionRegisterClient {
		s.register(m.Body)
		return
	}
	p := s.page
	if p == nil {
		return
	}
	switch m.Action {
	case protocol.ActionControlEvent:
		target, _ := protocol.ToInt64(m.Body["target"])
		name, _ := m.Body["name"].(string)
		data := m.Body["data"]
		s.post(func() { p.dispatch(target, name, data) })

	case protocol.ActionUpdateControlProps:
		// The client changed some properties, e.g. the text in a TextField.
		id, _ := protocol.ToInt64(m.Body["id"])
		props, _ := m.Body["props"].(map[string]any)
		s.post(func() { p.applyClientProps(id, props) })

	case protocol.ActionInvokeMethod:
		// Answered here, not in the queue: a handler may be waiting for it.
		p.handleMethodResult(m.Body)
	}
}

// register starts a new page, or reattaches the page of a known session.
func (s *session) register(body map[string]any) {
	sid, _ := body["session_id"].(string)
	info, _ := body["page"].(map[string]any)

	s.srv.mu.Lock()
	p := s.srv.pages[sid]
	isNew := p == nil
	if isNew {
		sid = randomID()
		p = newPage(s.send)
		s.srv.pages[sid] = p
	}
	s.srv.mu.Unlock()
	s.page = p

	s.send(protocol.Message{
		Action: protocol.ActionRegisterClient,
		Body: map[string]any{
			"session_id": sid,
			"page_patch": p.connect(s.send, info),
			"error":      "",
		},
	})
	if isNew {
		s.post(func() { s.srv.main(p) })
	}
}

// post runs fn on the page's queue. A panic is logged and shown in the client.
func (s *session) post(fn func()) {
	s.page.queue.post(func() {
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("%v\n\n%s", r, debug.Stack())
				log.Printf("flet: panic: %s", msg)
				s.send(protocol.Message{
					Action: protocol.ActionSessionCrashed,
					Body:   map[string]any{"message": msg},
				})
			}
		}()
		fn()
	})
}

// connect attaches the page to a connection and returns the full page state.
func (p *Page) connect(send func(protocol.Message), info map[string]any) map[string]any {
	p.mu.Lock()
	p.send = send
	p.ready = false
	if info != nil {
		p.root.applyProps(info)
		if win, ok := info["window"].(map[string]any); ok && p.Window != nil {
			p.Window.applyProps(win)
		}
	}
	// A reconnecting client starts from scratch: send everything again.
	p.sent = map[int64]*snapshot{}
	p.index = map[int64]Control{}
	p.mu.Unlock()
	return p.initial()
}

// disconnect detaches the page from its connection. Waiting method calls fail.
func (p *Page) disconnect() {
	p.mu.Lock()
	p.ready = false
	p.send = nil
	p.mu.Unlock()
	p.calls.failAll(ErrDisconnected)
}

// dispatch delivers a client event to its control.
func (p *Page) dispatch(target int64, name string, data any) {
	c := p.lookup(target)
	if c == nil {
		return
	}
	// Dialogs opened with ShowDialog leave the page once dismissed. Date and
	// time pickers send false when they were dismissed with a new value.
	if name == "dismiss" && p.dialogDismissed(c) && data != nil && data != true {
		return
	}
	c.handleEvent(&Event{Control: c, Name: name, Data: data, Page: p})
}
