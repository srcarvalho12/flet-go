// Contador: um texto e um botão "+1".
package main

import (
	"log"
	"strconv"

	flet "github.com/srcarvalho12/flet-go"
)

func main() {
	err := flet.App(func(page *flet.Page) {
		page.Title = "Contador"
		count := 0
		txt := &flet.Text{Value: "0", Size: 40}

		page.Add(&flet.Column{
			Controls: []flet.Control{
				txt,
				&flet.Button{
					Content: "+1",
					OnClick: func(e flet.Event) {
						count++
						txt.Value = strconv.Itoa(count)
						page.Update()
					},
				},
			},
		})
	})
	if err != nil {
		log.Fatal(err)
	}
}
