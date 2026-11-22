// Services: dialogs, a snack bar, focus, the clipboard, the file picker and
// window methods. Each button calls a method on the client and waits for it.
package main

import (
	"fmt"
	"log"
	"strings"

	flet "github.com/srcarvalho12/flet-go"
)

func main() {
	err := flet.App(func(page *flet.Page) {
		page.Title = "Services"

		clipboard := &flet.Clipboard{}
		picker := &flet.FilePicker{}
		page.Services = append(page.Services, clipboard, picker)

		status := &flet.Text{Value: "Ready"}
		show := func(format string, args ...any) {
			status.Value = fmt.Sprintf(format, args...)
			page.Update()
		}

		field := &flet.TextField{Label: "Some text", Value: "Hello from Go"}

		dialog := &flet.AlertDialog{
			Title:   "A dialog",
			Content: &flet.Text{Value: "Opened with page.ShowDialog."},
			OnDismiss: func(e *flet.Event) {
				show("Dialog dismissed")
			},
		}
		dialog.Actions = []flet.Control{
			&flet.TextButton{Content: "Close", OnClick: func(e *flet.Event) {
				page.PopDialog()
			}},
		}

		button := func(text string, onClick func()) *flet.Button {
			return &flet.Button{Content: text, OnClick: func(*flet.Event) { onClick() }}
		}

		page.Add(
			field,
			&flet.Row{Wrap: true, Controls: []flet.Control{
				button("Focus field", func() {
					if err := field.Focus(); err != nil {
						show("Focus: %v", err)
					}
				}),
				button("Copy", func() {
					if err := clipboard.Set(field.Value); err != nil {
						show("Copy: %v", err)
						return
					}
					show("Copied %q", field.Value)
				}),
				button("Paste", func() {
					v, err := clipboard.Get()
					if err != nil {
						show("Paste: %v", err)
						return
					}
					field.Value = v
					show("Pasted %q", v)
				}),
			}},
			&flet.Row{Wrap: true, Controls: []flet.Control{
				button("Dialog", func() {
					if err := page.ShowDialog(dialog); err != nil {
						show("Dialog: %v", err)
					}
				}),
				button("Snack bar", func() {
					page.ShowDialog(&flet.SnackBar{Content: "A snack bar from Go"})
				}),
				button("Pick files", func() {
					files, err := picker.PickFiles(flet.PickFilesOptions{AllowMultiple: true})
					if err != nil {
						show("Pick files: %v", err)
						return
					}
					var names []string
					for _, f := range files {
						names = append(names, f.Name)
					}
					show("Picked %d file(s): %s", len(files), strings.Join(names, ", "))
				}),
				button("Center window", func() {
					if err := page.Window.Center(); err != nil {
						show("Center: %v", err)
					}
				}),
			}},
			status,
		)
	})
	if err != nil {
		log.Fatal(err)
	}
}
