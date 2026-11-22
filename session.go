package flet

import (
	"log"
	"net"
	"sync"

	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// session is the conversation with one connected client.
type session struct {
	conn  net.Conn
	main  func(*Page)
	page  *Page
	debug bool
	wmu   sync.Mutex
}

func (s *session) send(m protocol.Message) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.debug {
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
		if s.debug {
			log.Printf("← action %d: %v", m.Action, m.Body)
		}
		s.handle(m)
	}
}

func (s *session) handle(m protocol.Message) {
	switch m.Action {
	case protocol.ActionRegisterClient:
		s.page = newPage(s)
		s.main(s.page)
		s.send(protocol.Message{
			Action: protocol.ActionRegisterClient,
			Body: map[string]any{
				"session_id": "go-session",
				"page_patch": s.page.initial(),
				"error":      "",
			},
		})

	case protocol.ActionControlEvent:
		if s.page == nil {
			return
		}
		target, _ := protocol.ToInt64(m.Body["target"])
		name, _ := m.Body["name"].(string)
		c := s.page.lookup(target)
		if c == nil {
			return
		}
		c.handleEvent(Event{Control: c, Name: name, Data: m.Body["data"], Page: s.page})

	case protocol.ActionUpdateControlProps:
		// E.g. window size. Not handled in the prototype yet.
	}
}
