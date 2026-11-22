// Package flet permite escrever interfaces em Go desenhadas pelo cliente Flet.
package flet

import "sync/atomic"

// Control é qualquer elemento da árvore de interface.
type Control interface {
	base() *controlBase
	// kind é o nome do controle no cliente Dart (ex.: "Text").
	kind() string
	// props são as propriedades simples (texto, número, bool, on_*).
	props() map[string]any
	// lists são as propriedades que guardam filhos (ex.: "controls").
	lists() map[string][]Control
	// handleEvent despacha um evento vindo do cliente.
	handleEvent(e Event)
}

// IDs 1 e 2 são reservados pelo cliente para Page e Window.
var idCounter atomic.Int64

func init() { idCounter.Store(2) }

type controlBase struct {
	id int64
}

func (b *controlBase) base() *controlBase {
	if b.id == 0 {
		b.id = idCounter.Add(1)
	}
	return b
}

// ID devolve o identificador do controle no protocolo.
func (b *controlBase) ID() int64 { return b.base().id }

func (b *controlBase) lists() map[string][]Control { return nil }
func (b *controlBase) handleEvent(Event)           {}

// encode transforma o controle (e seus filhos) no mapa enviado ao cliente.
func encode(c Control) map[string]any {
	m := map[string]any{
		"_c": c.kind(),
		"_i": c.base().id,
	}
	for k, v := range c.props() {
		m[k] = v
	}
	for k, children := range c.lists() {
		m[k] = encodeList(children)
	}
	return m
}

func encodeList(cs []Control) []any {
	out := make([]any, len(cs))
	for i, c := range cs {
		out[i] = encode(c)
	}
	return out
}

// setIf adiciona a propriedade só quando ela não é o valor zero,
// como o SDK Python faz (só envia o que difere do padrão).
func setIf[T comparable](m map[string]any, key string, v T) {
	var zero T
	if v != zero {
		m[key] = v
	}
}
