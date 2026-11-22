// List: add, remove and reorder items to exercise item-by-item list updates.
package main

import (
	"log"
	"slices"
	"strconv"

	flet "github.com/srcarvalho12/flet-go"
)

func main() {
	err := flet.App(func(page *flet.Page) {
		page.Title = "List"
		n := 0
		items := &flet.Column{}

		page.Add(
			&flet.Row{Controls: []flet.Control{
				&flet.Button{Content: "Add", OnClick: func(e flet.Event) {
					n++
					items.Controls = append(items.Controls, &flet.Text{Value: "Item " + strconv.Itoa(n)})
					page.Update()
				}},
				&flet.Button{Content: "Remove first", OnClick: func(e flet.Event) {
					if len(items.Controls) > 0 {
						items.Controls = items.Controls[1:]
						page.Update()
					}
				}},
				&flet.Button{Content: "Reverse", OnClick: func(e flet.Event) {
					slices.Reverse(items.Controls)
					page.Update()
				}},
			}},
			items,
		)
	})
	if err != nil {
		log.Fatal(err)
	}
}
