package flet

// The controls themselves are generated in controls_gen.go.
// These two are internal: the page root and its main view.

// view is the main screen inside the page.
type view struct {
	controlBase
	Controls []Control
}

func (v *view) kind() string                { return "View" }
func (v *view) props() map[string]any       { return map[string]any{} }
func (v *view) lists() map[string][]Control { return map[string][]Control{"controls": v.Controls} }

// pageRoot is the root control (fixed id 1 in the client).
type pageRoot struct {
	controlBase
	title string
	views []Control
}

func (p *pageRoot) kind() string { return "Page" }

func (p *pageRoot) props() map[string]any {
	m := map[string]any{}
	setIf(m, "title", p.title)
	return m
}

func (p *pageRoot) lists() map[string][]Control { return map[string][]Control{"views": p.views} }
