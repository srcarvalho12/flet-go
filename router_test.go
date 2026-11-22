package flet

import (
	"strings"
	"sync"
	"testing"
)

func texts(v *View) string {
	var out []string
	var walk func(c Control)
	walk = func(c Control) {
		if t, ok := c.(*Text); ok {
			out = append(out, t.Value)
		}
		children(c, walk)
	}
	walk(v)
	return strings.Join(out, ",")
}

func textPage(s string) PageFunc {
	return func(ctx *RouteContext) Control { return &Text{Value: s} }
}

func TestRouterMatch(t *testing.T) {
	r := NewRouter()
	r.Route("/", textPage("home"))
	r.Route("/products/{id}", textPage("product"))
	r.Route("/products/new", textPage("new"))
	r.Route("/files/{path...}", textPage("files"))
	cases := []struct{ path, want, param string }{
		{"/", "/", ""},
		{"/products/42", "/products/{id}", "42"},
		{"/products/new", "/products/new", ""},
		{"/products/42/", "/products/{id}", "42"},
		{"/files/a/b/c.txt", "/files/{path...}", "a/b/c.txt"},
		{"/products", "", ""},
		{"/products/a%20b", "/products/{id}", "a b"},
	}
	for _, c := range cases {
		rt, params := r.match(c.path)
		got := ""
		if rt != nil {
			got = rt.pattern
		}
		if got != c.want {
			t.Errorf("%s matched %q, want %q", c.path, got, c.want)
			continue
		}
		if c.param != "" && params["id"] != c.param && params["path"] != c.param {
			t.Errorf("%s params %v, want %q", c.path, params, c.param)
		}
	}
}

func TestRouterLayouts(t *testing.T) {
	layout := func(name string) LayoutFunc {
		return func(ctx *RouteContext, content Control) *View {
			return &View{Controls: []Control{&Text{Value: name}, content}}
		}
	}
	r := NewRouter()
	r.Layout(layout("main"))
	r.Route("/", textPage("home"))
	r.Route("/login", textPage("login"), NoLayout())
	r.Route("/special", textPage("special"), WithLayout(layout("special")))
	r.Route("/full", func(ctx *RouteContext) Control {
		return &View{AppBar: &AppBar{Title: "own"}, Controls: []Control{&Text{Value: "full"}}}
	})
	admin := r.Group("/admin", layout("admin"))
	admin.Route("/", textPage("dashboard"))
	admin.Route("/users/{id}", func(ctx *RouteContext) Control {
		return &Text{Value: "user " + ctx.Param("id") + " tab " + ctx.Query.Get("tab")}
	})
	r.NotFound(textPage("missing"))

	p, fc := newTestPage(t)
	connect(p, fc)
	r.Main(p)

	cases := []struct{ route, want string }{
		{"/", "main,home"},
		{"/login", "login"},
		{"/special", "special,special"},
		{"/full", "full"},
		{"/admin", "admin,dashboard"},
		{"/admin/users/7?tab=roles", "admin,user 7 tab roles"},
		{"/nope", "main,missing"},
	}
	for _, c := range cases {
		p.Go(c.route)
		if len(p.Views) != 1 {
			t.Fatalf("%s: %d views", c.route, len(p.Views))
		}
		if got := texts(p.Views[0]); got != c.want {
			t.Errorf("%s shows %q, want %q", c.route, got, c.want)
		}
		if p.Views[0].Route != c.route || p.Route != c.route {
			t.Errorf("%s: view route %q, page route %q", c.route, p.Views[0].Route, p.Route)
		}
		assertInSync(t, p, fc)
	}
	if want := "push_route map[route:/admin/users/7?tab=roles]"; !strings.Contains(strings.Join(fc.calls, "\n"), want) {
		t.Errorf("address not updated, calls: %v", fc.calls)
	}
}

func TestRouterStacked(t *testing.T) {
	builds := map[string]int{}
	page := func(s string) PageFunc {
		return func(ctx *RouteContext) Control { builds[s]++; return &Text{Value: s} }
	}
	r := NewRouter()
	r.Route("/", page("home"))
	products := r.Group("/products", nil, Stacked())
	products.Route("/", page("list"))
	products.Route("/{id}", page("detail"))

	p, fc := newTestPage(t)
	connect(p, fc)
	r.Main(p)

	p.Go("/products")
	home := p.Views[0]
	p.Go("/products/42")
	var got []string
	for _, v := range p.Views {
		got = append(got, v.Route+"="+texts(v))
	}
	if strings.Join(got, " ") != "/=home /products=list /products/42=detail" {
		t.Fatalf("stack: %v", got)
	}
	if p.Views[0] != home || builds["home"] != 1 || builds["list"] != 1 {
		t.Errorf("screens underneath were rebuilt: %v", builds)
	}
	assertInSync(t, p, fc)

	// The back arrow pops the top view: the page below is shown.
	p.dispatch(1, "view_pop", map[string]any{"route": "/products/42"})
	if len(p.Views) != 2 || p.Route != "/products" {
		t.Fatalf("after pop: %d views, route %q", len(p.Views), p.Route)
	}
	assertInSync(t, p, fc)

	// A deep link builds the whole stack.
	p2, fc2 := newTestPage(t)
	connect(p2, fc2)
	p2.Route = "/products/7"
	r.Main(p2)
	if len(p2.Views) != 3 {
		t.Fatalf("deep link: %d views", len(p2.Views))
	}
}

func TestRouterRedirectAndRouteChange(t *testing.T) {
	loggedIn := false
	requireLogin := func(next PageFunc) PageFunc {
		return func(ctx *RouteContext) Control {
			if !loggedIn {
				ctx.Redirect("/login")
				return nil
			}
			return next(ctx)
		}
	}
	r := NewRouter()
	r.Route("/login", textPage("login"))
	private := r.Group("/", nil)
	private.Use(requireLogin)
	private.Route("/", textPage("home"))

	p, fc := newTestPage(t)
	connect(p, fc)
	changes := 0
	p.OnRouteChange = func(e *RouteChangeEvent) { changes++ }
	r.Main(p)
	if p.Route != "/login" || texts(p.Views[0]) != "login" {
		t.Fatalf("not redirected: %q %q", p.Route, texts(p.Views[0]))
	}

	// The browser's back/forward buttons and address bar send route_change.
	loggedIn = true
	p.dispatch(1, "route_change", map[string]any{"route": "/"})
	if texts(p.Views[0]) != "home" || changes != 1 {
		t.Fatalf("route change: %q, %d handler calls", texts(p.Views[0]), changes)
	}
	// The same route again is ignored (the client echoes Go calls).
	p.dispatch(1, "route_change", map[string]any{"route": "/"})
	if changes != 1 {
		t.Errorf("duplicate route change was handled")
	}
	assertInSync(t, p, fc)
}

func TestPageProps(t *testing.T) {
	p, fc := newTestPage(t)
	connect(p, fc)
	p.ThemeMode = ThemeModeDark
	p.Theme = &Theme{ColorSchemeSeed: "green"}
	p.Views[0].AppBar = &AppBar{Title: "App"}
	p.Views[0].Bgcolor = "blue"
	p.Update()
	assertInSync(t, p, fc)

	resized := false
	p.OnResize = func(e *PageResizeEvent) { resized = e.Width == 1024 }
	p.dispatch(1, "resize", map[string]any{"width": 1024.0, "height": 768.0})
	if !resized || p.Width != 1024 || p.Height != 768 {
		t.Fatalf("resize: handler=%v size=%vx%v", resized, p.Width, p.Height)
	}
}

// Run serializes work from other goroutines with the event handlers.
func TestPageRun(t *testing.T) {
	p, fc := newTestPage(t)
	connect(p, fc)
	count := &Text{Value: ""}
	p.Add(count)
	n := 0
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			done := make(chan struct{})
			p.Run(func() {
				n++
				count.Value = strings.Repeat("x", n)
				p.Update()
				close(done)
			})
			<-done
		}()
	}
	wg.Wait()
	if n != 50 {
		t.Fatalf("n = %d", n)
	}
	assertInSync(t, p, fc)
}

func TestSessionKey(t *testing.T) {
	type cart struct{ items int }
	key := NewSessionKey(func() *cart { return &cart{} })
	user := NewSessionKey[string](nil)
	a, _ := newTestPage(t)
	b, _ := newTestPage(t)

	key.Get(a).items++
	key.Get(a).items++
	if key.Get(a).items != 2 || key.Get(b).items != 0 {
		t.Fatalf("values are not per page: %d %d", key.Get(a).items, key.Get(b).items)
	}
	if user.Get(a) != "" {
		t.Fatal("zero value expected")
	}
	user.Set(a, "ana")
	if user.Get(a) != "ana" || user.Get(b) != "" {
		t.Fatal("Set is not per page")
	}
}
