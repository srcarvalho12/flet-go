// Package pages holds the screens of the router example, one file per area.
package pages

import (
	flet "github.com/srcarvalho12/flet-go"
	"github.com/srcarvalho12/flet-go/icons"
)

// MainLayout is the app bar and navigation bar shared by most pages.
func MainLayout(ctx *flet.RouteContext, content flet.Control) *flet.View {
	tabs := []string{"/", "/products", "/admin"}
	selected := 0
	for i, t := range tabs {
		if ctx.Path == t {
			selected = i
		}
	}
	return &flet.View{
		AppBar: &flet.AppBar{
			Title:   "Shop",
			Leading: &flet.Image{Src: "logo.svg", Width: 32, Height: 32},
		},
		NavigationBar: &flet.NavigationBar{
			SelectedIndex: selected,
			Destinations: []flet.Control{
				&flet.NavigationBarDestination{Icon: icons.Home, Label: "Home"},
				&flet.NavigationBarDestination{Icon: icons.ShoppingBag, Label: "Products"},
				&flet.NavigationBarDestination{Icon: icons.AdminPanelSettings, Label: "Admin"},
			},
			OnChange: func(e *flet.Event) {
				ctx.Go(tabs[e.Control.(*flet.NavigationBar).SelectedIndex])
			},
		},
		Padding:  20,
		Controls: []flet.Control{content},
	}
}

// NotFound is shown for unknown routes.
func NotFound(ctx *flet.RouteContext) flet.Control {
	return &flet.Text{Value: "Nothing at " + ctx.Path}
}
