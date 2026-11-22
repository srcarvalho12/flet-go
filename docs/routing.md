# Routing

The router lets each page live in its own file, connected to a route in
`main.go`. The browser's address bar, back button and deep links work on their own.

```
myapp/
  main.go          → routes
  pages/
    layout.go      → app bar and menu shared by pages
    home.go
    products.go
    admin.go
```

## Routes

```go
func main() {
	r := flet.NewRouter()
	r.Layout(pages.MainLayout)
	r.Route("/", pages.Home)
	r.Route("/products", pages.Products)
	r.Route("/products/{id}", pages.Product)
	r.NotFound(pages.NotFound)

	flet.Serve(":8550", r.Main) // or flet.App(r.Main)
}
```

To set up the page first (title, theme), call `r.Main` from your own function:

```go
flet.Serve(":8550", func(page *flet.Page) {
	page.Theme = &flet.Theme{ColorSchemeSeed: "indigo"}
	r.Main(page)
})
```

## Pages

A page is a function that receives a `*flet.RouteContext` and returns a control.
It runs every time its route opens.

```go
func Product(ctx *flet.RouteContext) flet.Control {
	id := ctx.Param("id")          // from "/products/{id}"
	tab := ctx.Query.Get("tab")    // from "?tab=reviews"
	return &flet.Column{Controls: []flet.Control{
		&flet.Text{Value: "Product " + id + " " + tab},
		&flet.Button{Content: "Back", OnClick: func(*flet.Event) {
			ctx.Go("/products")
		}},
	}}
}
```

| Field or method | Meaning |
|---|---|
| `ctx.Page` | The page |
| `ctx.Route` | Full route, e.g. `/products/42?tab=reviews` |
| `ctx.Path` | Route without the query |
| `ctx.Param(name)` | A path parameter |
| `ctx.Query` | Query parameters (`url.Values`) |
| `ctx.Go(route)` | Navigate (same as `page.Go`) |
| `ctx.Redirect(route)` | Show another route instead (return `nil` after it) |

A page can return a `*flet.View` to control the whole screen (its own app bar,
navigation bar...). The layout is skipped then.

## Patterns

Patterns follow `net/http`:

| Pattern | Matches |
|---|---|
| `/products` | Only `/products` |
| `/products/{id}` | `/products/42`, with `id` = `42` |
| `/files/{path...}` | `/files/a/b.txt`, with `path` = `a/b.txt` |

When several patterns match, a fixed segment beats a parameter, which beats
`{name...}`. So `/products/new` wins over `/products/{id}`.

## Layouts

A layout wraps the content of a page in a screen:

```go
func MainLayout(ctx *flet.RouteContext, content flet.Control) *flet.View {
	return &flet.View{
		AppBar:   &flet.AppBar{Title: "Shop"},
		Padding:  20,
		Controls: []flet.Control{content},
	}
}
```

You can have as many layouts as you need. The most specific one wins:

```go
r.Layout(pages.MainLayout)                          // default

admin := r.Group("/admin", pages.AdminLayout)       // everything under /admin
admin.Route("/", pages.Dashboard)                   // /admin
admin.Route("/users", pages.Users)                  // /admin/users

r.Route("/login", pages.Login, flet.NoLayout())     // no layout at all
r.Route("/promo", pages.Promo, flet.WithLayout(pages.PromoLayout))
```

Order: the route's own option, then the nearest group, then the router.

## Middleware

Middleware runs before the pages of a group, for example to require a login:

```go
var user = flet.NewSessionKey[string](nil)

func RequireLogin(next flet.PageFunc) flet.PageFunc {
	return func(ctx *flet.RouteContext) flet.Control {
		if user.Get(ctx.Page) == "" {
			ctx.Redirect("/login?next=" + ctx.Path)
			return nil
		}
		return next(ctx)
	}
}

admin := r.Group("/admin", pages.AdminLayout)
admin.Use(RequireLogin)
```

## Stacked screens

On mobile, screens usually stack: the detail opens on top of the list, with a back
arrow. Turn it on for a group or a route with `flet.Stacked()`:

```go
products := r.Group("/products", nil, flet.Stacked())
products.Route("/", pages.Products)     // /products
products.Route("/{id}", pages.Product)  // /products/42
```

Each route is registered once. The stack is built from the address: opening
`/products/42` shows `/products` underneath (and `/` under it, when it is a route).
The back arrow goes to the screen below. Screens already on the stack are kept,
not rebuilt, so going deeper animates only the new screen. Deep links build the
whole stack.

On the web, routes are usually not stacked: each address shows one screen and the
browser keeps the history.

## Navigation events

`page.OnRouteChange` and `page.OnViewPop` still run when you use the router, after
it has shown the new route.

See [`examples/router`](../examples/router) for a complete app.
