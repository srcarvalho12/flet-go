package flet

import (
	"fmt"
	"log"
	"runtime/debug"
	"slices"

	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// Run runs fn on the page's event queue, one at a time with the event
// handlers. Use it to change controls from another goroutine (a timer, a
// chat message, a background job), so it never runs at the same time as a
// handler:
//
//	go func() {
//		for msg := range messages {
//			page.Run(func() {
//				list.Controls = append(list.Controls, &flet.Text{Value: msg})
//				page.Update()
//			})
//		}
//	}()
//
// A panic in fn is logged and shown in the client.
func (p *Page) Run(fn func()) {
	p.queue.post(func() {
		defer p.recoverPanic()
		fn()
	})
}

func (p *Page) recoverPanic() {
	r := recover()
	if r == nil {
		return
	}
	msg := fmt.Sprintf("%v\n\n%s", r, debug.Stack())
	log.Printf("flet: panic: %s", msg)
	p.mu.Lock()
	send := p.send
	p.mu.Unlock()
	if send != nil {
		send(protocol.Message{
			Action: protocol.ActionSessionCrashed,
			Body:   map[string]any{"message": msg},
		})
	}
}

// Go navigates to a route, e.g. "/products/42". With a Router, the route's
// page is shown right away; the browser address and history follow.
// Without one, the client reports the change in OnRouteChange.
func (p *Page) Go(route string) {
	final := p.navigate(route)
	p.pushRoute(final)
}

// navigate shows a route with the router, following redirects, and returns
// the route that was shown.
func (p *Page) navigate(route string) string {
	p.mu.Lock()
	r := p.router
	p.mu.Unlock()
	if r == nil {
		return route
	}
	for range 10 {
		p.mu.Lock()
		old := slices.Clone(p.Views)
		p.mu.Unlock()
		views, redirect := r.render(p, route, old)
		if redirect == "" {
			p.mu.Lock()
			p.Views = views
			p.Route = route
			p.lastRoute = route
			p.mu.Unlock()
			p.Update()
			return route
		}
		route = redirect
	}
	log.Printf("flet: too many redirects, stopped at %q", route)
	return route
}

// pushRoute updates the address shown by the client.
func (p *Page) pushRoute(route string) {
	p.mu.Lock()
	p.lastRoute = route
	p.Route = route
	p.mu.Unlock()
	if _, err := p.root.invoke("push_route", map[string]any{"route": route}); err != nil &&
		err != ErrDisconnected && err != ErrNotMounted {
		log.Printf("flet: push route %q: %v", route, err)
	}
}

// handlePageEvent handles events of the page itself, then calls the
// handlers in PageProps.
func (p *Page) handlePageEvent(e *Event) {
	switch e.Name {
	case "route_change":
		ev := &RouteChangeEvent{}
		decodeEvent(e, ev)
		p.mu.Lock()
		dup := ev.Route == p.lastRoute
		p.lastRoute, p.Route = ev.Route, ev.Route
		hasRouter := p.router != nil
		p.mu.Unlock()
		if dup {
			return
		}
		if hasRouter {
			if final := p.navigate(ev.Route); final != ev.Route {
				p.pushRoute(final)
			}
		}

	case "view_pop":
		ev := &ViewPopEvent{}
		decodeEvent(e, ev)
		p.mu.Lock()
		hasRouter := p.router != nil
		prev := ""
		for i, v := range p.Views {
			if v != nil && v.Route == ev.Route && i > 0 && p.Views[i-1] != nil {
				prev = p.Views[i-1].Route
			}
		}
		p.mu.Unlock()
		if hasRouter && prev != "" {
			p.Go(prev)
		}

	case "resize":
		ev := &PageResizeEvent{}
		decodeEvent(e, ev)
		p.mu.Lock()
		p.Width, p.Height = ev.Width, ev.Height
		p.mu.Unlock()

	case "platform_brightness_change":
		ev := &PlatformBrightnessChangeEvent{}
		decodeEvent(e, ev)
		p.mu.Lock()
		p.PlatformBrightness = ev.Brightness
		p.mu.Unlock()
	}
	p.PageProps.handleEvent(e)
}
