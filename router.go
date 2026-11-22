package flet

import (
	"net/url"
	"path"
	"strings"
)

// PageFunc builds the content of a route. It runs every time the route is
// opened. It may return a *View to control the whole screen (app bar,
// navigation bar...); otherwise the layout wraps what it returns.
type PageFunc func(ctx *RouteContext) Control

// LayoutFunc wraps the content of a route in a screen, e.g. to add an app
// bar or a navigation bar shared by several routes.
type LayoutFunc func(ctx *RouteContext, content Control) *View

// Middleware runs around a PageFunc, e.g. to require a login:
//
//	func requireLogin(next flet.PageFunc) flet.PageFunc {
//		return func(ctx *flet.RouteContext) flet.Control {
//			if !loggedIn(ctx.Page) {
//				ctx.Redirect("/login")
//				return nil
//			}
//			return next(ctx)
//		}
//	}
type Middleware func(next PageFunc) PageFunc

// RouteContext is what a page receives: the page, the route and its
// parameters.
type RouteContext struct {
	// Page is the page being shown.
	Page *Page
	// Route is the full route, e.g. "/products/42?tab=reviews".
	Route string
	// Path is the route without the query, e.g. "/products/42".
	Path string
	// Query holds the query parameters, e.g. Query.Get("tab").
	Query url.Values

	params   map[string]string
	redirect string
}

// Param returns a path parameter, e.g. "id" for the pattern "/products/{id}".
func (c *RouteContext) Param(name string) string { return c.params[name] }

// Go navigates to another route.
func (c *RouteContext) Go(route string) { c.Page.Go(route) }

// Redirect shows another route instead of this one. Call it from a page or a
// middleware and return nil.
func (c *RouteContext) Redirect(route string) { c.redirect = route }

// RouteOption configures a route or a group.
type RouteOption func(*routeOptions)

type routeOptions struct {
	layout   LayoutFunc
	noLayout bool
	stacked  *bool
}

// WithLayout gives a route or group its own layout.
func WithLayout(l LayoutFunc) RouteOption {
	return func(o *routeOptions) { o.layout, o.noLayout = l, false }
}

// NoLayout shows a route or group without any layout.
func NoLayout() RouteOption {
	return func(o *routeOptions) { o.layout, o.noLayout = nil, true }
}

// Stacked shows a route on top of its parent routes, with a back arrow:
// opening /products/42 shows /products underneath, and / under it, when
// those routes exist. Typical on mobile; on the web each route usually
// replaces the previous one.
func Stacked() RouteOption {
	t := true
	return func(o *routeOptions) { o.stacked = &t }
}

// Router maps routes to pages. Pass Router.Main to App or Serve:
//
//	r := flet.NewRouter()
//	r.Route("/", pages.Home)
//	r.Route("/products/{id}", pages.Product)
//	flet.Serve(":8550", r.Main)
//
// Patterns use "{name}" for one path segment and "{name...}" for the rest of
// the path, like net/http.
type Router struct {
	RouteGroup
	notFound PageFunc
	routes   []*route
}

// RouteGroup is a set of routes under a path prefix that share a layout,
// middleware and options.
type RouteGroup struct {
	router *Router
	parent *RouteGroup
	prefix string
	opts   routeOptions
	mw     []Middleware
}

type route struct {
	pattern  string
	segments []string
	page     PageFunc
	group    *RouteGroup
	opts     routeOptions
}

// NewRouter returns an empty router.
func NewRouter() *Router {
	r := &Router{}
	r.RouteGroup.router = r
	return r
}

// Layout sets the layout of the group (for a Router: the default layout).
func (g *RouteGroup) Layout(l LayoutFunc) { g.opts.layout, g.opts.noLayout = l, false }

// Use adds middleware to every route of the group, in order.
func (g *RouteGroup) Use(mw ...Middleware) { g.mw = append(g.mw, mw...) }

// Route adds a page for a pattern, relative to the group prefix.
func (g *RouteGroup) Route(pattern string, page PageFunc, opts ...RouteOption) {
	rt := &route{pattern: joinPath(g.fullPrefix(), pattern), page: page, group: g}
	rt.segments = splitPath(rt.pattern)
	for _, o := range opts {
		o(&rt.opts)
	}
	g.router.routes = append(g.router.routes, rt)
}

// Group adds a group of routes under prefix. layout may be nil to keep the
// parent's layout.
func (g *RouteGroup) Group(prefix string, layout LayoutFunc, opts ...RouteOption) *RouteGroup {
	sub := &RouteGroup{router: g.router, parent: g, prefix: prefix}
	if layout != nil {
		sub.opts.layout = layout
	}
	for _, o := range opts {
		o(&sub.opts)
	}
	return sub
}

// NotFound sets the page shown when no route matches.
func (r *Router) NotFound(page PageFunc) { r.notFound = page }

// Main shows the current route and follows route changes. Pass it to App
// or Serve.
func (r *Router) Main(page *Page) {
	page.mu.Lock()
	page.router = r
	route := page.Route
	page.mu.Unlock()
	page.Go(route)
}

func (g *RouteGroup) fullPrefix() string {
	if g.parent == nil {
		return joinPath("/", g.prefix)
	}
	return joinPath(g.parent.fullPrefix(), g.prefix)
}

// layout returns the layout for a route: the route's own, then the nearest
// group's. ok is false when the route is shown without a layout.
func (rt *route) layout() (LayoutFunc, bool) {
	if rt.opts.noLayout {
		return nil, false
	}
	if rt.opts.layout != nil {
		return rt.opts.layout, true
	}
	for g := rt.group; g != nil; g = g.parent {
		if g.opts.noLayout {
			return nil, false
		}
		if g.opts.layout != nil {
			return g.opts.layout, true
		}
	}
	return nil, false
}

func (rt *route) stacked() bool {
	if rt.opts.stacked != nil {
		return *rt.opts.stacked
	}
	for g := rt.group; g != nil; g = g.parent {
		if g.opts.stacked != nil {
			return *g.opts.stacked
		}
	}
	return false
}

// handler returns the page wrapped by the middleware of its groups,
// outermost group first.
func (rt *route) handler() PageFunc {
	h := rt.page
	for g := rt.group; g != nil; g = g.parent {
		for i := len(g.mw) - 1; i >= 0; i-- {
			h = g.mw[i](h)
		}
	}
	return h
}

// match finds the most specific route for a path: at each segment a fixed
// name beats "{param}", which beats "{rest...}".
func (r *Router) match(p string) (*route, map[string]string) {
	segs := splitPath(p)
	var best *route
	var bestParams map[string]string
	var bestScore []int
	for _, rt := range r.routes {
		params, score, ok := matchSegments(rt.segments, segs)
		if !ok {
			continue
		}
		if best == nil || better(score, bestScore) {
			best, bestParams, bestScore = rt, params, score
		}
	}
	return best, bestParams
}

func matchSegments(pattern, segs []string) (map[string]string, []int, bool) {
	params := map[string]string{}
	var score []int
	for i, ps := range pattern {
		if name, ok := strings.CutSuffix(strings.Trim(ps, "{}"), "..."); ok && isParam(ps) {
			params[name] = strings.Join(segs[min(i, len(segs)):], "/")
			return params, append(score, 1), true
		}
		if i >= len(segs) {
			return nil, nil, false
		}
		switch {
		case isParam(ps):
			params[strings.Trim(ps, "{}")] = segs[i]
			score = append(score, 2)
		case ps == segs[i]:
			score = append(score, 3)
		default:
			return nil, nil, false
		}
	}
	if len(pattern) != len(segs) {
		return nil, nil, false
	}
	return params, score, true
}

func better(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return len(a) > len(b)
}

func isParam(s string) bool { return strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") }

func splitPath(p string) []string {
	p = strings.Trim(path.Clean("/"+p), "/")
	if p == "" {
		return nil
	}
	parts := strings.Split(p, "/")
	for i, s := range parts {
		if u, err := url.PathUnescape(s); err == nil && !isParam(s) {
			parts[i] = u
		}
	}
	return parts
}

func joinPath(a, b string) string { return path.Clean("/" + a + "/" + b) }

// render builds the views for a route. Ancestor views that are already on
// screen are kept, so pushing a stacked route only adds the new screen.
func (r *Router) render(page *Page, full string, old []*View) (views []*View, redirect string) {
	u, err := url.Parse(full)
	if err != nil {
		u = &url.URL{Path: "/"}
	}
	p := path.Clean("/" + u.Path)

	rt, _ := r.match(p)
	paths := []string{p}
	if rt != nil && rt.stacked() {
		segs := splitPath(p)
		paths = paths[:0]
		for i := 0; i < len(segs); i++ {
			ancestor := "/" + strings.Join(segs[:i], "/")
			if a, _ := r.match(ancestor); a != nil {
				paths = append(paths, ancestor)
			}
		}
		paths = append(paths, p)
	}

	for i, vp := range paths {
		top := i == len(paths)-1
		route, query := vp, url.Values{}
		if top {
			route, query = full, u.Query()
		}
		// Keep an ancestor that is already on screen for the same route.
		if !top && i < len(old) && old[i] != nil && old[i].Route == route {
			views = append(views, old[i])
			continue
		}
		v, redirect := r.build(page, route, vp, query)
		if redirect != "" {
			return nil, redirect
		}
		views = append(views, v)
	}
	return views, ""
}

// build runs the page (and its layout) for one path.
func (r *Router) build(page *Page, route, p string, query url.Values) (*View, string) {
	rt, params := r.match(p)
	ctx := &RouteContext{Page: page, Route: route, Path: p, Query: query, params: params}

	var content Control
	var layout LayoutFunc
	hasLayout := false
	switch {
	case rt != nil:
		content = rt.handler()(ctx)
		layout, hasLayout = rt.layout()
	case r.notFound != nil:
		content = r.notFound(ctx)
		layout, hasLayout = r.opts.layout, r.opts.layout != nil && !r.opts.noLayout
	default:
		content = &Text{Value: "Page not found: " + p}
	}
	if ctx.redirect != "" {
		return nil, ctx.redirect
	}

	var v *View
	switch c := content.(type) {
	case *View:
		v = c
	default:
		if hasLayout {
			v = layout(ctx, content)
		}
		if v == nil {
			v = &View{}
			if content != nil {
				v.Controls = []Control{content}
			}
		}
	}
	v.Route = route
	return v, ""
}
