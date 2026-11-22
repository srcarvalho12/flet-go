// Router: pages in their own files, a shared layout, a stacked group, a
// protected admin area and the app's own assets. Run it and open
// http://localhost:8550 (or pass -desktop to open a window).
package main

import (
	"flag"
	"log"

	flet "github.com/srcarvalho12/flet-go"
	"github.com/srcarvalho12/flet-go/examples/router/pages"
)

func main() {
	addr := flag.String("addr", ":8550", "address to listen on")
	desktop := flag.Bool("desktop", false, "open a desktop window instead")
	flag.Parse()

	r := flet.NewRouter()
	r.Layout(pages.MainLayout)
	r.Route("/", pages.Home)
	r.Route("/login", pages.Login, flet.NoLayout())
	r.NotFound(pages.NotFound)

	// /products/42 opens on top of /products, with a back arrow.
	products := r.Group("/products", nil, flet.Stacked())
	products.Route("/", pages.Products)
	products.Route("/{id}", pages.Product)

	admin := r.Group("/admin", pages.AdminLayout)
	admin.Use(pages.RequireLogin)
	admin.Route("/", pages.Dashboard)

	main := func(page *flet.Page) {
		page.Title = "Shop"
		page.Theme = &flet.Theme{ColorSchemeSeed: "indigo"}
		r.Main(page)
	}

	var err error
	if *desktop {
		err = flet.App(main, flet.AppOptions{AssetsDir: "assets"})
	} else {
		err = flet.Serve(*addr, main, flet.WebOptions{AssetsDir: "assets"})
	}
	if err != nil {
		log.Fatal(err)
	}
}
