package flet

// Event is an event coming from the client, such as a click.
type Event struct {
	// Control is the control that fired the event.
	Control Control
	// Name is the event name without the "on_" prefix (e.g. "click").
	Name string
	// Data holds extra event data, when there is any.
	Data any
	// Page is the page the event belongs to.
	Page *Page
}
