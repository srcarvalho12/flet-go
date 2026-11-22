// Package flet lets you write user interfaces in Go, rendered by the Flet client.
package flet

import "sync/atomic"

// Control is any element of the UI tree.
type Control interface {
	base() *controlBase
	// kind is the control name in the Dart client (e.g. "Text").
	kind() string
	// props are the plain properties (text, numbers, styles, on_* flags).
	props() map[string]any
	// lists are the properties that hold a list of children (e.g. "controls").
	lists() map[string][]Control
	// slots are the properties that hold a single child (e.g. "content").
	slots() map[string]Control
	// handleEvent dispatches an event coming from the client.
	handleEvent(e *Event)
}

// IDs 1 and 2 are reserved by the client for Page and Window.
var idCounter atomic.Int64

func init() { idCounter.Store(2) }

type controlBase struct {
	id int64
}

func (b *controlBase) base() *controlBase {
	if b.id == 0 {
		b.id = idCounter.Add(1)
	}
	return b
}

// ID returns the control identifier used by the protocol.
func (b *controlBase) ID() int64 { return b.base().id }

func (b *controlBase) lists() map[string][]Control { return nil }
func (b *controlBase) slots() map[string]Control   { return nil }
func (b *controlBase) handleEvent(*Event)          {}

// encode turns the control (and its children) into the map sent to the client.
func encode(c Control) map[string]any {
	m := map[string]any{
		"_c": c.kind(),
		"_i": c.base().id,
	}
	for k, v := range c.props() {
		m[k] = v
	}
	for k, children := range c.lists() {
		m[k] = encodeList(children)
	}
	for k, child := range c.slots() {
		m[k] = encode(child)
	}
	return m
}

func encodeList(cs []Control) []any {
	out := make([]any, len(cs))
	for i, c := range cs {
		out[i] = encode(c)
	}
	return out
}

// children calls fn for every direct child of c.
func children(c Control, fn func(Control)) {
	for _, list := range c.lists() {
		for _, child := range list {
			fn(child)
		}
	}
	for _, child := range c.slots() {
		fn(child)
	}
}

// setIf adds the property only when it is not the zero value,
// like the Python SDK does (it only sends what differs from the default).
func setIf[T comparable](m map[string]any, key string, v T) {
	var zero T
	if v != zero {
		m[key] = v
	}
}
