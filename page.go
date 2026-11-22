package flet

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// Page is the app window. It holds the control tree and sends changes.
type Page struct {
	// Title is the window title.
	Title string
	// Window is the native window (desktop only): size, position, methods
	// such as Center and Close.
	Window *Window
	// Overlay holds controls drawn above the page content.
	Overlay []Control
	// Services holds non-visual controls such as Clipboard or FilePicker.
	// A service must be on the page (followed by Update) before its methods
	// are called.
	Services []Control

	// Reported by the client.
	Width, Height      float64
	Platform           PagePlatform
	PlatformBrightness Brightness
	Web                bool
	Route              string
	// ClientIP and ClientUserAgent identify the browser (web only).
	ClientIP, ClientUserAgent string

	// OnConnect is called when the client reconnects to this page.
	OnConnect func(e *Event)
	// OnDisconnect is called when the connection drops.
	OnDisconnect func(e *Event)
	// OnClose is called when a disconnected session expires (web only).
	// Use it to stop goroutines started for this page.
	OnClose func(e *Event)

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

	calls   calls
	queue   queue
	uploads *uploads
}

type snapshot struct {
	props map[string]any
	lists map[string][]int64
	slots map[string]int64
}

func newPage(send func(protocol.Message)) *Page {
	v := &view{}
	w := &Window{}
	w.id = 2
	p := &Page{
		Window: w,
		send:   send,
		view:   v,
		sent:   map[int64]*snapshot{},
		index:  map[int64]Control{},
	}
	p.root = &pageRoot{
		controlBase: controlBase{id: 1},
		page:        p,
		views:       []Control{v},
		window:      w,
		overlay:     &overlay{},
		dialogs:     &dialogs{},
		services:    &serviceRegistry{uid: randomID()},
	}
	return p
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

// ShowDialog opens a dialog (AlertDialog, BottomSheet, SnackBar, DatePicker...).
// The dialog is removed from the page once the client reports it was dismissed.
func (p *Page) ShowDialog(d Control) error {
	dc, ok := d.(dialogControl)
	if !ok {
		return fmt.Errorf("flet: %s is not a dialog", d.kind())
	}
	p.mu.Lock()
	if slices.Contains(p.root.dialogs.controls, d) {
		p.mu.Unlock()
		return errors.New("flet: dialog is already open")
	}
	*dc.openField() = true
	p.root.dialogs.controls = append(p.root.dialogs.controls, d)
	p.mu.Unlock()
	p.Update()
	return nil
}

// PopDialog closes the most recently opened dialog and returns it,
// or returns nil when no dialog is open.
func (p *Page) PopDialog() Control {
	p.mu.Lock()
	var found dialogControl
	for i := len(p.root.dialogs.controls) - 1; i >= 0; i-- {
		if dc, ok := p.root.dialogs.controls[i].(dialogControl); ok && *dc.openField() {
			found = dc
			break
		}
	}
	if found == nil {
		p.mu.Unlock()
		return nil
	}
	*found.openField() = false
	p.mu.Unlock()
	p.Update()
	return found
}

// dialogDismissed removes a dialog the client closed. It reports whether c
// was a dialog opened with ShowDialog.
func (p *Page) dialogDismissed(c Control) bool {
	p.mu.Lock()
	i := slices.Index(p.root.dialogs.controls, c)
	if i < 0 {
		p.mu.Unlock()
		return false
	}
	p.root.dialogs.controls = slices.Delete(p.root.dialogs.controls, i, i+1)
	p.mu.Unlock()
	p.Update()
	return true
}

// Update sends the client only what changed since the last update.
func (p *Page) Update() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.ready {
		return
	}
	p.sync()
	p.diff(p.root)
	if p.pruneNeeded {
		p.prune()
	}
}

// initial builds the full page state for the client registration.
func (p *Page) initial() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sync()
	m := encode(p.root)
	p.record(p.root)
	p.ready = true
	return m
}

// sync copies the exported page fields into the internal tree.
func (p *Page) sync() {
	p.root.title = p.Title
	if p.Window != nil {
		p.Window.id = 2
		p.root.window = p.Window
	}
	p.root.overlay.controls = p.Overlay
	p.root.services.services = p.Services
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
	b.page.Store(p)
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
			p.index[id].base().page.CompareAndSwap(p, nil)
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
