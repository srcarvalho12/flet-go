package flet

// Event é um evento vindo do cliente, como um clique.
type Event struct {
	// Control é o controle que gerou o evento.
	Control Control
	// Name é o nome do evento sem o prefixo "on_" (ex.: "click").
	Name string
	// Data traz dados extras do evento, quando houver.
	Data any
	// Page é a página do evento.
	Page *Page
}
