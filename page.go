package flet

import (
	"reflect"
	"sync"

	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// Page é a janela do app. Guarda a árvore de controles e envia as mudanças.
type Page struct {
	// Title é o título da janela.
	Title string

	mu    sync.Mutex
	s     *session
	root  *pageRoot
	view  *view
	ready bool

	// sent guarda o último estado enviado de cada controle, para o diff.
	sent map[int64]*snapshot
	// index liga o id de cada controle ao controle, para despachar eventos.
	index map[int64]Control
}

type snapshot struct {
	props map[string]any
	lists map[string][]int64
}

func newPage(s *session) *Page {
	v := &view{}
	root := &pageRoot{controlBase: controlBase{id: 1}, views: []Control{v}}
	return &Page{
		s:     s,
		root:  root,
		view:  v,
		sent:  map[int64]*snapshot{},
		index: map[int64]Control{},
	}
}

// Add adiciona controles à tela e atualiza a página.
func (p *Page) Add(cs ...Control) {
	p.mu.Lock()
	p.view.Controls = append(p.view.Controls, cs...)
	p.mu.Unlock()
	p.Update()
}

// Update envia ao cliente só o que mudou desde o último envio.
func (p *Page) Update() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.ready {
		return
	}
	p.root.title = p.Title
	p.diff(p.root)
}

// initial monta o estado completo da página para o registro do cliente.
func (p *Page) initial() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.root.title = p.Title
	m := encode(p.root)
	p.record(p.root)
	p.ready = true
	return m
}

func (p *Page) lookup(id int64) Control {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.index[id]
}

// record guarda o estado atual de um controle e de todos os seus filhos.
func (p *Page) record(c Control) {
	b := c.base()
	snap := &snapshot{props: c.props(), lists: map[string][]int64{}}
	for k, children := range c.lists() {
		snap.lists[k] = idsOf(children)
		for _, child := range children {
			p.record(child)
		}
	}
	p.sent[b.id] = snap
	p.index[b.id] = c
}

// diff compara um controle com o último envio e manda as operações de patch.
func (p *Page) diff(c Control) {
	id := c.base().id
	snap, ok := p.sent[id]
	if !ok {
		// Não deveria acontecer: controles novos entram pelo pai.
		p.record(c)
		return
	}

	var ops []any

	// Propriedades simples.
	cur := c.props()
	for k, v := range cur {
		if old, ok := snap.props[k]; !ok || !reflect.DeepEqual(old, v) {
			ops = append(ops, []any{protocol.OpReplace, 0, k, v})
		}
	}
	for k := range snap.props {
		if _, ok := cur[k]; !ok {
			ops = append(ops, []any{protocol.OpReplace, 0, k, nil})
		}
	}
	snap.props = cur

	// Listas de filhos: se a lista mudou, envia a lista inteira.
	var unchanged [][]Control
	for k, children := range c.lists() {
		ids := idsOf(children)
		if equalIDs(ids, snap.lists[k]) {
			unchanged = append(unchanged, children)
			continue
		}
		ops = append(ops, []any{protocol.OpReplace, 0, k, encodeList(children)})
		for _, child := range children {
			p.record(child)
		}
		snap.lists[k] = ids
	}

	if len(ops) > 0 {
		patch := append([]any{[]any{0}}, ops...)
		p.s.send(protocol.Message{
			Action: protocol.ActionPatchControl,
			Body:   map[string]any{"id": id, "patch": patch},
		})
	}

	for _, children := range unchanged {
		for _, child := range children {
			p.diff(child)
		}
	}
}

func idsOf(cs []Control) []int64 {
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.base().id
	}
	return ids
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
