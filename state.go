package flet

import "sync"

// SessionKey stores a typed value per page (per browser tab on the web),
// shared by every route of the session, e.g. the logged-in user or a cart:
//
//	var cart = flet.NewSessionKey(func() *Cart { return &Cart{} })
//
//	func Checkout(ctx *flet.RouteContext) flet.Control {
//		c := cart.Get(ctx.Page)
//		...
//	}
type SessionKey[T any] struct {
	init func() T
}

// NewSessionKey returns a key whose value starts as init() on each page.
// init may be nil to start from the zero value.
func NewSessionKey[T any](init func() T) *SessionKey[T] {
	return &SessionKey[T]{init: init}
}

// Get returns the page's value, creating it on first use.
func (k *SessionKey[T]) Get(p *Page) T {
	p.values.mu.Lock()
	defer p.values.mu.Unlock()
	if v, ok := p.values.m[k]; ok {
		return v.(T)
	}
	var v T
	if k.init != nil {
		v = k.init()
	}
	if p.values.m == nil {
		p.values.m = map[any]any{}
	}
	p.values.m[k] = v
	return v
}

// Set replaces the page's value.
func (k *SessionKey[T]) Set(p *Page, v T) {
	p.values.mu.Lock()
	defer p.values.mu.Unlock()
	if p.values.m == nil {
		p.values.m = map[any]any{}
	}
	p.values.m[k] = v
}

type sessionValues struct {
	mu sync.Mutex
	m  map[any]any
}
