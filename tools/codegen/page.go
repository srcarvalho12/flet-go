package main

import (
	"path/filepath"
	"strings"
)

// pageSkip lists Page fields that are not user settings: the view stack and
// window (managed by Page), values reported by the client (read-only on Page)
// and handlers that Page implements by hand or that need features not
// supported yet.
var pageSkip = map[string]bool{
	"views": true, "multi_views": true, "window": true, "title": true, "route": true,
	"width": true, "height": true, "media": true, "embedded": true, "web": true,
	"pwa": true, "debug": true, "wasm": true, "test": true, "multi_view": true,
	"pyodide": true, "platform_brightness": true, "client_ip": true,
	"client_user_agent": true, "platform": true,
	"on_connect": true, "on_disconnect": true, "on_close": true,
	"on_login": true, "on_logout": true,
	"on_multi_view_add": true, "on_multi_view_remove": true,
}

// emitPage writes PageProps: the settings and event handlers of the page
// (theme, fonts, on_resize...), embedded in the hand-written Page.
func (m *model) emitPage(out string) error {
	var fs []*field
	for _, f := range m.fields("Page", map[string]bool{"Control": true, "BaseControl": true}) {
		if !pageSkip[f.Name] {
			fs = append(fs, f)
		}
	}

	w := &writer{}
	w.p(header+"package flet\n\nimport (\n\"maps\"\n\"slices\"\n\"time\"\n)\n\nvar _ = maps.Clone[map[string]any]\nvar _ = slices.Clone[[]string]\nvar _ time.Duration", m.spec.FletVersion)
	w.p("// PageProps are the page settings and events, embedded in Page.")
	w.p("type PageProps struct {")
	for _, f := range fs {
		if f.typ.kind == kControl || f.typ.kind == kControls {
			continue
		}
		fieldDecl(w, f, false)
	}
	w.p("}")
	w.p("")

	w.p("func (c *PageProps) props() map[string]any {")
	w.p("m := map[string]any{}")
	for _, f := range fs {
		if f.typ.kind != kControl && f.typ.kind != kControls {
			emitEncode(w, "c", f, f.Name)
		}
	}
	w.p("return m")
	w.p("}")
	w.p("")

	w.p("func (c *PageProps) handleEvent(e *Event) {")
	w.p("switch e.Name {")
	for _, f := range fs {
		if f.typ.kind != kHandler {
			continue
		}
		w.p("case %q:", strings.TrimPrefix(f.Name, "on_"))
		w.p("if c.%s != nil {", f.goName)
		if f.typ.event == "" {
			w.p("c.%s(e)", f.goName)
		} else {
			w.p("ev := &%s{Event: *e}", f.typ.event)
			w.p("decodeEvent(e, ev)")
			w.p("c.%s(ev)", f.goName)
		}
		w.p("}")
	}
	w.p("}")
	w.p("}")
	return writeGo(filepath.Join(out, "page_gen.go"), w)
}
