package pages

import (
	"fmt"
	"strconv"

	flet "github.com/srcarvalho12/flet-go"
)

// cart is kept per browser tab and shared by every page.
var cart = flet.NewSessionKey(func() *[]string { return &[]string{} })

func cartSummary(p *flet.Page) string {
	return fmt.Sprintf("%d item(s)", len(*cart.Get(p)))
}

var products = []string{"Keyboard", "Mouse", "Monitor"}

// Products lists the products. It runs inside the stacked group, so it gets a
// full screen of its own (no shared layout) with a back arrow to Home.
func Products(ctx *flet.RouteContext) flet.Control {
	list := &flet.Column{}
	for i, name := range products {
		list.Controls = append(list.Controls, &flet.ListTile{
			Title: name,
			OnClick: func(*flet.Event) {
				ctx.Go("/products/" + strconv.Itoa(i))
			},
		})
	}
	return &flet.View{
		AppBar:   &flet.AppBar{Title: "Products"},
		Controls: []flet.Control{list},
	}
}

// Product shows one product, on top of the list.
func Product(ctx *flet.RouteContext) flet.Control {
	i, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || i < 0 || i >= len(products) {
		return NotFound(ctx)
	}
	name := products[i]
	status := &flet.Text{Value: "Cart: " + cartSummary(ctx.Page)}
	return &flet.View{
		AppBar: &flet.AppBar{Title: name},
		Controls: []flet.Control{
			&flet.Text{Value: name, Size: 28},
			&flet.Button{Content: "Add to cart", OnClick: func(*flet.Event) {
				c := cart.Get(ctx.Page)
				*c = append(*c, name)
				status.Value = "Cart: " + cartSummary(ctx.Page)
				ctx.Page.Update()
			}},
			status,
		},
	}
}
