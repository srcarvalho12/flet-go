// Web: the counter served to the browser. Open http://localhost:8550 in one
// or more tabs; each tab has its own count.
package main

import (
	"log"
	"strconv"

	flet "github.com/srcarvalho12/flet-go"
	"github.com/srcarvalho12/flet-go/icons"
)

func main() {
	err := flet.Serve(":8550", func(page *flet.Page) {
		page.Title = "Web counter"

		count := 0
		value := &flet.Text{Value: "0", Size: 40}
		change := func(delta int) func(e *flet.Event) {
			return func(e *flet.Event) {
				count += delta
				value.Value = strconv.Itoa(count)
				page.Update()
			}
		}

		page.Add(
			&flet.Row{
				Alignment:         flet.MainAxisAlignmentCenter,
				VerticalAlignment: flet.CrossAxisAlignmentCenter,
				Controls: []flet.Control{
					&flet.IconButton{Icon: icons.Remove, OnClick: change(-1)},
					value,
					&flet.IconButton{Icon: icons.Add, OnClick: change(+1)},
				},
			},
			&flet.Text{Value: "Platform: " + string(page.Platform) + " · browser: " + page.ClientUserAgent},
		)
	})
	if err != nil {
		log.Fatal(err)
	}
}
