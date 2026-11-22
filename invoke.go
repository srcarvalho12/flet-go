package flet

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/srcarvalho12/flet-go/internal/protocol"
	"github.com/vmihailenco/msgpack/v5"
)

var (
	// ErrNotMounted is returned when a method is called on a control that is
	// not on a page (add it and call Update first).
	ErrNotMounted = errors.New("flet: control is not on the page")
	// ErrDisconnected is returned for method calls that were still waiting
	// when the client disconnected.
	ErrDisconnected = errors.New("flet: client disconnected")
)

// MethodError is an error reported by the client for a method call.
type MethodError struct {
	Control string
	Method  string
	Message string
}

func (e *MethodError) Error() string {
	return fmt.Sprintf("flet: %s.%s: %s", e.Control, e.Method, e.Message)
}

type callResult struct {
	value any
	err   error
}

// calls tracks method calls waiting for the client's answer.
type calls struct {
	mu      sync.Mutex
	pending map[string]chan callResult
}

func (c *calls) add(id string) chan callResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		c.pending = map[string]chan callResult{}
	}
	ch := make(chan callResult, 1)
	c.pending[id] = ch
	return ch
}

// resolve delivers an answer. Answers nobody waits for are dropped.
func (c *calls) resolve(id string, r callResult) {
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		ch <- r
	}
}

// failAll ends every waiting call with err.
func (c *calls) failAll(err error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = nil
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- callResult{err: err}
	}
}

// invoke calls a method of a control on the client and waits for the result.
func (p *Page) invoke(c Control, name string, args map[string]any) (any, error) {
	p.mu.Lock()
	send, ready := p.send, p.ready
	p.mu.Unlock()
	if !ready || send == nil {
		return nil, ErrDisconnected
	}
	id := c.base().id
	callID := randomID()
	ch := p.calls.add(callID)
	send(protocol.Message{
		Action: protocol.ActionInvokeMethod,
		Body: map[string]any{
			"control_id": id,
			"call_id":    callID,
			"name":       name,
			"args":       args,
		},
	})
	r := <-ch
	if r.err != nil {
		var me *MethodError
		if errors.As(r.err, &me) {
			me.Control, me.Method = c.kind(), name
		}
		return nil, r.err
	}
	return r.value, nil
}

// handleMethodResult is called by the session when the client answers.
func (p *Page) handleMethodResult(body map[string]any) {
	callID, _ := body["call_id"].(string)
	r := callResult{value: body["result"]}
	if msg, _ := body["error"].(string); msg != "" {
		r.err = &MethodError{Message: msg}
	}
	p.calls.resolve(callID, r)
}

// invoke calls a client method of the control.
func (b *controlBase) invoke(name string, args map[string]any) (any, error) {
	p := b.page.Load()
	if p == nil {
		return nil, ErrNotMounted
	}
	p.mu.Lock()
	c := p.index[b.id]
	p.mu.Unlock()
	if c == nil {
		return nil, ErrNotMounted
	}
	return p.invoke(c, name, args)
}

// invokeResult calls a client method and decodes its result into T.
func invokeResult[T any](b *controlBase, name string, args map[string]any) (T, error) {
	var out T
	v, err := b.invoke(name, args)
	if err != nil || v == nil {
		return out, err
	}
	if x, ok := v.(T); ok {
		return x, nil
	}
	data, err := msgpack.Marshal(v)
	if err != nil {
		return out, err
	}
	if err := msgpack.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("flet: %s: unexpected result %v: %w", name, v, err)
	}
	return out, nil
}

// encodeArg encodes a method argument like a property value.
func encodeArg(v any) any {
	switch x := v.(type) {
	case nil, string, bool, int, float64:
		return x
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() != reflect.Uint8 {
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = encodeArg(rv.Index(i).Interface())
		}
		return out
	}
	return encodeAny(v)
}

func randomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// queue runs functions one at a time, in order, on its own goroutine.
// Event handlers run there, so they may call client methods and wait for the
// answer while the session keeps reading messages.
type queue struct {
	mu      sync.Mutex
	items   []func()
	running bool
}

func (q *queue) post(fn func()) {
	q.mu.Lock()
	q.items = append(q.items, fn)
	if q.running {
		q.mu.Unlock()
		return
	}
	q.running = true
	q.mu.Unlock()
	go q.loop()
}

func (q *queue) loop() {
	for {
		q.mu.Lock()
		if len(q.items) == 0 {
			q.running = false
			q.mu.Unlock()
			return
		}
		fn := q.items[0]
		q.items = q.items[1:]
		q.mu.Unlock()
		fn()
	}
}
