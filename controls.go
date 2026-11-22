package flet

// The controls themselves are generated in controls_gen.go.
// These are internal: the page root and the containers the client expects
// under the page.

// overlay holds the controls drawn above the page content.
type overlay struct {
	controlBase
	controls []Control
}

func (o *overlay) kind() string { return "Overlay" }
func (o *overlay) props() map[string]any {
	return map[string]any{"_internals": map[string]any{"host_positioned": true}}
}
func (o *overlay) lists() map[string][]Control { return map[string][]Control{"controls": o.controls} }

// dialogs holds the dialogs opened with Page.ShowDialog.
type dialogs struct {
	controlBase
	controls []Control
}

func (d *dialogs) kind() string                { return "Dialogs" }
func (d *dialogs) props() map[string]any       { return map[string]any{} }
func (d *dialogs) lists() map[string][]Control { return map[string][]Control{"controls": d.controls} }

// serviceRegistry holds the services (Clipboard, FilePicker...) of a page.
type serviceRegistry struct {
	controlBase
	uid      string
	services []Control
}

func (s *serviceRegistry) kind() string { return "ServiceRegistry" }
func (s *serviceRegistry) props() map[string]any {
	return map[string]any{"_internals": map[string]any{"uid": s.uid}}
}
func (s *serviceRegistry) lists() map[string][]Control {
	return map[string][]Control{"_services": s.services}
}

// pageRoot is the root control (fixed id 1 in the client).
type pageRoot struct {
	controlBase
	page     *Page
	title    string
	views    []Control
	window   *Window
	overlay  *overlay
	dialogs  *dialogs
	services *serviceRegistry
}

func (r *pageRoot) kind() string { return "Page" }

func (r *pageRoot) props() map[string]any {
	m := r.page.PageProps.props()
	setIf(m, "title", r.title)
	return m
}

func (r *pageRoot) handleEvent(e *Event) { r.page.handlePageEvent(e) }

func (r *pageRoot) lists() map[string][]Control { return map[string][]Control{"views": r.views} }

func (r *pageRoot) slots() map[string]Control {
	return map[string]Control{
		"window":    r.window,
		"_overlay":  r.overlay,
		"_dialogs":  r.dialogs,
		"_services": r.services,
	}
}

// applyProps stores page properties reported by the client (size, platform...).
func (r *pageRoot) applyProps(p map[string]any) {
	pg := r.page
	for k, v := range p {
		switch k {
		case "width":
			assign(&pg.Width, v)
		case "height":
			assign(&pg.Height, v)
		case "platform":
			assign(&pg.Platform, v)
		case "web":
			assign(&pg.Web, v)
		case "platform_brightness":
			assign(&pg.PlatformBrightness, v)
		case "route":
			assign(&pg.Route, v)
		}
	}
}

// dialogControl is implemented by the generated dialog controls
// (AlertDialog, BottomSheet, SnackBar...).
type dialogControl interface {
	Control
	openField() *bool
}
