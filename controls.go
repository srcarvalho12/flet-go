package flet

// Text mostra um texto.
type Text struct {
	controlBase
	Value string
	Size  float64
	Color string
}

func (t *Text) kind() string { return "Text" }

func (t *Text) props() map[string]any {
	m := map[string]any{}
	setIf(m, "value", t.Value)
	setIf(m, "size", t.Size)
	setIf(m, "color", t.Color)
	return m
}

// Button é um botão com texto.
type Button struct {
	controlBase
	Content string
	OnClick func(e Event)
}

func (b *Button) kind() string { return "Button" }

func (b *Button) props() map[string]any {
	m := map[string]any{}
	setIf(m, "content", b.Content)
	if b.OnClick != nil {
		m["on_click"] = true
	}
	return m
}

func (b *Button) handleEvent(e Event) {
	if e.Name == "click" && b.OnClick != nil {
		b.OnClick(e)
	}
}

// Column empilha controles na vertical.
type Column struct {
	controlBase
	Controls []Control
}

func (c *Column) kind() string                { return "Column" }
func (c *Column) props() map[string]any       { return map[string]any{} }
func (c *Column) lists() map[string][]Control { return map[string][]Control{"controls": c.Controls} }

// Row alinha controles na horizontal.
type Row struct {
	controlBase
	Controls []Control
}

func (r *Row) kind() string                { return "Row" }
func (r *Row) props() map[string]any       { return map[string]any{} }
func (r *Row) lists() map[string][]Control { return map[string][]Control{"controls": r.Controls} }

// view é a tela principal dentro da página.
type view struct {
	controlBase
	Controls []Control
}

func (v *view) kind() string                { return "View" }
func (v *view) props() map[string]any       { return map[string]any{} }
func (v *view) lists() map[string][]Control { return map[string][]Control{"controls": v.Controls} }

// pageRoot é o controle raiz (id fixo 1 no cliente).
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
