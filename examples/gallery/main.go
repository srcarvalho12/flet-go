// Gallery: a sample of generated controls, styles, colors, icons and inputs.
package main

import (
	"fmt"
	"log"

	flet "github.com/srcarvalho12/flet-go"
	"github.com/srcarvalho12/flet-go/colors"
	"github.com/srcarvalho12/flet-go/icons"
)

func main() {
	err := flet.App(func(page *flet.Page) {
		page.Title = "Gallery"

		greeting := &flet.Text{Value: "Type your name above", Italic: true}
		name := &flet.TextField{
			Label: "Your name",
			OnChange: func(e *flet.Event) {
				greeting.Value = "Hello, " + e.Control.(*flet.TextField).Value + "!"
				page.Update()
			},
		}

		status := &flet.Text{Value: "Checkbox: off · Switch: off"}
		check := &flet.Checkbox{Label: "Check me"}
		toggle := &flet.Switch{Label: "Switch me"}
		updateStatus := func(e *flet.Event) {
			status.Value = fmt.Sprintf("Checkbox: %s · Switch: %s", onOff(check.Value), onOff(toggle.Value))
			page.Update()
		}
		check.OnChange = updateStatus
		toggle.OnChange = updateStatus

		progress := &flet.ProgressBar{Value: 0.3, Width: 300}
		sliderLabel := &flet.Text{Value: "Slider: 30"}
		slider := &flet.Slider{
			Value: 30,
			Max:   flet.Float(100),
			Width: 300,
			OnChange: func(e *flet.Event) {
				v := e.Control.(*flet.Slider).Value
				sliderLabel.Value = fmt.Sprintf("Slider: %.0f", v)
				progress.Value = v / 100
				page.Update()
			},
		}

		choice := &flet.Text{Value: "Pick a color"}
		dropdown := &flet.Dropdown{
			Label: "Color",
			Width: 300,
			Options: []flet.Control{
				&flet.DropdownOption{Text: "Red"},
				&flet.DropdownOption{Text: "Green"},
				&flet.DropdownOption{Text: "Blue"},
			},
			OnSelect: func(e *flet.Event) {
				choice.Value = "Selected: " + e.Control.(*flet.Dropdown).Value
				page.Update()
			},
		}

		likes := 0
		likesText := &flet.Text{Value: "0 likes"}

		page.Add(
			&flet.Text{Value: "flet-go gallery", Size: 28, Weight: flet.FontWeightBold},
			&flet.Container{
				Padding:      16,
				BorderRadius: 12,
				Bgcolor:      colors.BlueGrey100,
				Content: &flet.Row{Controls: []flet.Control{
					&flet.Icon{Icon: icons.Favorite, Color: colors.Red},
					&flet.Text{Value: "A Container with padding, radius and a background color"},
				}},
			},
			name, greeting,
			check, toggle, status,
			slider, sliderLabel, progress,
			dropdown, choice,
			&flet.Row{Controls: []flet.Control{
				&flet.IconButton{Icon: icons.ThumbUp, IconColor: colors.Blue, OnClick: func(e *flet.Event) {
					likes++
					likesText.Value = fmt.Sprintf("%d likes", likes)
					page.Update()
				}},
				likesText,
			}},
			&flet.Card{Content: &flet.ListTile{
				Leading:  &flet.Icon{Icon: icons.Person},
				Title:    "A Card with a ListTile",
				Subtitle: "Title and subtitle are plain strings",
			}},
		)
	})
	if err != nil {
		log.Fatal(err)
	}
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
