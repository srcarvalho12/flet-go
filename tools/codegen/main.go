// Command codegen generates the Go controls, value types, events, enums,
// colors and icons from spec/flet.json (made by tools/specgen/extract.py).
package main

import (
	"flag"
	"log"
	"path/filepath"
)

func main() {
	specPath := flag.String("spec", "spec/flet.json", "spec file")
	out := flag.String("out", ".", "output directory (module root)")
	report := flag.Bool("report", false, "print skipped fields")
	flag.Parse()

	s, err := loadSpec(*specPath)
	if err != nil {
		log.Fatal(err)
	}
	m := newModel(s)
	steps := []func() error{
		func() error { return m.emitEnums(*out) },
		func() error { return m.emitValues(*out) },
		func() error { return m.emitEvents(*out) },
		func() error { return m.emitControls(*out) },
		func() error { return m.emitMethods(*out, m.allMethods()) },
		func() error { return m.emitPage(*out) },
		func() error { return m.emitColors(*out) },
		func() error { return m.emitIcons(filepath.Dir(*specPath), *out) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			log.Fatal(err)
		}
	}
	if *report {
		m.printReport()
	}
}
