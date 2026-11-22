// Upload: pick files in the browser and upload them to the server, served
// under a base path. Open http://localhost:8550/app/ and the files are saved
// in the "uploads" folder.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	flet "github.com/srcarvalho12/flet-go"
)

func main() {
	addr := flag.String("addr", ":8550", "address to listen on")
	flag.Parse()

	opts := flet.WebOptions{BasePath: "/app/", UploadDir: "uploads"}
	err := flet.Serve(*addr, func(page *flet.Page) {
		page.Title = "Upload"

		results := &flet.Column{}
		picker := &flet.FilePicker{}
		picker.OnUpload = func(e *flet.FilePickerUploadEvent) {
			switch {
			case e.Error != "":
				results.Controls = append(results.Controls, &flet.Text{Value: e.FileName + ": " + e.Error})
			case e.Progress == 1:
				results.Controls = append(results.Controls, &flet.Text{Value: e.FileName + ": uploaded"})
			}
			page.Update()
		}
		page.Services = append(page.Services, picker)

		page.Add(
			&flet.Button{Content: "Pick and upload files", OnClick: func(*flet.Event) {
				files, err := picker.PickFiles(flet.PickFilesOptions{AllowMultiple: true})
				if err != nil || len(files) == 0 {
					return
				}
				var uploads []*flet.FilePickerUploadFile
				for _, f := range files {
					url, err := page.GetUploadURL(f.Name, time.Minute)
					if err != nil {
						results.Controls = append(results.Controls, &flet.Text{Value: err.Error()})
						continue
					}
					uploads = append(uploads, &flet.FilePickerUploadFile{ID: f.ID, Name: f.Name, UploadURL: url})
				}
				results.Controls = append(results.Controls, &flet.Text{Value: fmt.Sprintf("Uploading %d file(s)...", len(uploads))})
				page.Update()
				if err := picker.Upload(uploads); err != nil {
					results.Controls = append(results.Controls, &flet.Text{Value: err.Error()})
					page.Update()
				}
			}},
			results,
		)
	}, opts)
	if err != nil {
		log.Fatal(err)
	}
}
