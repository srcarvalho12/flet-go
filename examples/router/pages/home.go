package pages

import flet "github.com/srcarvalho12/flet-go"

// Home is the start page.
func Home(ctx *flet.RouteContext) flet.Control {
	return &flet.Column{Controls: []flet.Control{
		&flet.Text{Value: "Welcome", Size: 28},
		&flet.Text{Value: "Cart: " + cartSummary(ctx.Page)},
		&flet.Button{Content: "See products", OnClick: func(*flet.Event) { ctx.Go("/products") }},
	}}
}
