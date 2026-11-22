package flet

import (
	"reflect"
	"slices"
	"sync"

	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// Page is the app window. It holds the control tree and sends changes.
type Page struct {
	// Title is the window title.
	Title string

	mu    sync.Mutex
	send  func(protocol.Message)
	root  *pageRoot
	view  *view
	ready bool

	// sent holds the last state sent for each control, used for diffing.
	sent map[int64]*snapshot
	// index maps each control id to its control, used to dispatch events.
	index map[int64]Control
	// pruneNeeded is set when controls were removed during an update.
	pruneNeeded bool
}

type snapshot struct {
	props map[string]any
	lists map[string][]int64
	slots map[string]int64
}

func newPage(send func(protocol.Message)) *Page {
	v := &view{}
	root := &pageRoot{controlBase: controlBase{id: 1}, views: []Control{v}}
	return &Page{
		send:  send,
		root:  root,
		view:  v,
		sent:  map[int64]*snapshot{},
		index: map[int64]Control{},
	}
}

// Add appends controls to the screen and updates the page.
func (p *Page) Add(cs ...Control) {
	p.mu.Lock()
	p.view.Controls = append(p.view.Controls, cs...)
	p.mu.Unlock()
	p.Update()
}

// Remove removes controls from the screen and updates the page.
func (p *Page) Remove(cs ...Control) {
	p.mu.Lock()
	p.view.Controls = slices.DeleteFunc(p.view.Controls, func(c Control) bool {
		return slices.Contains(cs, c)
	})
	p.mu.Unlock()
	p.Update()
}

// Clean removes all controls from the screen and updates the page.
func (p *Page) Clean() {
	p.mu.Lock()
	p.view.Controls = nil
	p.mu.Unlock()
	p.Update()
}

// Update sends the client only what changed since the last update.
func (p *Page) Update() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.ready {
		return
	}
	p.root.title = p.Title
	p.diff(p.root)
	if p.pruneNeeded {
		p.prune()
	}
}

// initial builds the full page state for the client registration.
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

// applyClientProps applies properties changed on the client (e.g. text typed
// into a TextField) and marks them as already sent, so they are not echoed.
func (p *Page) applyClientProps(id int64, props map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.index[id]
	ps, ok := c.(propsSetter)
	if !ok {
		return
	}
	ps.applyProps(props)
	snap := p.sent[id]
	if snap == nil {
		return
	}
	cur := c.props()
	for k := range props {
		if v, ok := cur[k]; ok {
			snap.props[k] = v
		} else {
			delete(snap.props, k)
		}
	}
}

// record stores the current state of a control and all its children.
func (p *Page) record(c Control) {
	b := c.base()
	snap := &snapshot{props: c.props(), lists: map[string][]int64{}, slots: map[string]int64{}}
	for k, children := range c.lists() {
		snap.lists[k] = idsOf(children)
		for _, child := range children {
			p.record(child)
		}
	}
	for k, child := range c.slots() {
		snap.slots[k] = child.base().id
		p.record(child)
	}
	p.sent[b.id] = snap
	p.index[b.id] = c
}

// prune drops controls that are no longer in the tree from the
// diff state and the event index.
func (p *Page) prune() {
	alive := map[int64]bool{}
	var walk func(c Control)
	walk = func(c Control) {
		alive[c.base().id] = true
		children(c, walk)
	}
	walk(p.root)
	for id := range p.index {
		if !alive[id] {
			delete(p.index, id)
			delete(p.sent, id)
		}
	}
	p.pruneNeeded = false
}

// patch builds the patch for a single control.
//
// Format: [tree, op1, op2, ...], where tree is [0, {"prop": [n]}] and
// node 0 is the control itself; node n is the list stored in "prop".
type patch struct {
	children map[string]any
	ops      []any
	next     int
}

// listNode returns the tree node index for the list in property key.
func (pt *patch) listNode(key string) int {
	if pt.children == nil {
		pt.children = map[string]any{}
	}
	if n, ok := pt.children[key]; ok {
		return n.([]any)[0].(int)
	}
	pt.next++
	pt.children[key] = []any{pt.next}
	return pt.next
}

func (pt *patch) message() []any {
	tree := []any{0}
	if pt.children != nil {
		tree = append(tree, pt.children)
	}
	return append([]any{tree}, pt.ops...)
}

// diff compares a control with the last update and sends patch operations.
func (p *Page) diff(c Control) {
	id := c.base().id
	snap, ok := p.sent[id]
	if !ok {
		// Should not happen: new controls are added through their parent.
		p.record(c)
		return
	}

	pt := &patch{}

	// Simple properties.
	cur := c.props()
	for k, v := range cur {
		if old, ok := snap.props[k]; !ok || !reflect.DeepEqual(old, v) {
			pt.ops = append(pt.ops, []any{protocol.OpReplace, 0, k, v})
		}
	}
	for k := range snap.props {
		if _, ok := cur[k]; !ok {
			pt.ops = append(pt.ops, []any{protocol.OpReplace, 0, k, nil})
		}
	}
	snap.props = cur

	// Child lists, item by item.
	var existing []Control
	for k, children := range c.lists() {
		existing = append(existing, p.diffList(pt, k, snap.lists[k], children)...)
		snap.lists[k] = idsOf(children)
	}

	// Single children: replaced whole when a different control is set.
	slots := c.slots()
	for k := range snap.slots {
		if _, ok := slots[k]; !ok {
			// A field that now holds a plain value already got its op above.
			if _, isProp := cur[k]; !isProp {
				pt.ops = append(pt.ops, []any{protocol.OpReplace, 0, k, nil})
			}
			p.pruneNeeded = true
		}
	}
	newSlots := make(map[string]int64, len(slots))
	for k, child := range slots {
		cid := child.base().id
		newSlots[k] = cid
		if snap.slots[k] == cid {
			existing = append(existing, child)
			continue
		}
		if snap.slots[k] != 0 {
			p.pruneNeeded = true
		}
		pt.ops = append(pt.ops, []any{protocol.OpReplace, 0, k, encode(child)})
		p.record(child)
	}
	snap.slots = newSlots

	if len(pt.ops) > 0 {
		p.send(protocol.Message{
			Action: protocol.ActionPatchControl,
			Body:   map[string]any{"id": id, "patch": pt.message()},
		})
	}

	for _, child := range existing {
		p.diff(child)
	}
}

// diffList adds the operations that turn the old list (by id) into the new
// one. It returns the children that were already on the client, which must
// be diffed on their own.
func (p *Page) diffList(pt *patch, key string, old []int64, children []Control) []Control {
	next := idsOf(children)
	if slices.Equal(old, next) {
		return children
	}

	keep := map[int64]bool{}
	for _, id := range next {
		keep[id] = true
	}

	work := slices.Clone(old)
	var existing []Control

	// 1. Remove what is gone, from the end so indexes stay valid.
	for i := len(work) - 1; i >= 0; i-- {
		if !keep[work[i]] {
			pt.ops = append(pt.ops, []any{protocol.OpRemove, pt.listNode(key), i})
			work = slices.Delete(work, i, i+1)
			p.pruneNeeded = true
		}
	}

	// 2. Walk the new list, moving or adding items into place.
	for i, child := range children {
		id := next[i]
		if i < len(work) && work[i] == id {
			existing = append(existing, child)
			continue
		}
		if j := indexFrom(work, id, i+1); j >= 0 {
			n := pt.listNode(key)
			pt.ops = append(pt.ops, []any{protocol.OpMove, n, j, n, i})
			work = slices.Delete(work, j, j+1)
			work = slices.Insert(work, i, id)
			existing = append(existing, child)
			continue
		}
		pt.ops = append(pt.ops, []any{protocol.OpAdd, pt.listNode(key), i, encode(child)})
		work = slices.Insert(work, i, id)
		p.record(child)
	}
	return existing
}

func indexFrom(s []int64, v int64, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] == v {
			return i
		}
	}
	return -1
}

func idsOf(cs []Control) []int64 {
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.base().id
	}
	return ids
}
