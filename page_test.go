package flet

import (
	"math/rand"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/srcarvalho12/flet-go/internal/protocol"
)

// fakeClient mirrors how the Dart client applies patches
// (Control.applyPatch in packages/flet/lib/src/models/control.dart).
type fakeClient struct {
	t    *testing.T
	root map[string]any
	msgs int
}

// toClient copies a value, turning lists into *[]any so they can be mutated.
func toClient(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = toClient(val)
		}
		return m
	case []any:
		l := make([]any, len(x))
		for i, val := range x {
			l[i] = toClient(val)
		}
		return &l
	}
	return v
}

// fromClient converts back and drops nil properties, so the result can be
// compared with encode().
func fromClient(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, val := range x {
			if val != nil {
				m[k] = fromClient(val)
			}
		}
		return m
	case *[]any:
		l := make([]any, len(*x))
		for i, val := range *x {
			l[i] = fromClient(val)
		}
		return l
	}
	return v
}

func (fc *fakeClient) find(v any, id int64) map[string]any {
	switch x := v.(type) {
	case map[string]any:
		if x["_i"] == id {
			return x
		}
		for _, val := range x {
			if m := fc.find(val, id); m != nil {
				return m
			}
		}
	case *[]any:
		for _, val := range *x {
			if m := fc.find(val, id); m != nil {
				return m
			}
		}
	}
	return nil
}

func (fc *fakeClient) apply(msg protocol.Message) {
	fc.t.Helper()
	fc.msgs++
	if msg.Action != protocol.ActionPatchControl {
		fc.t.Fatalf("unexpected action %d", msg.Action)
	}
	id := msg.Body["id"].(int64)
	ctl := fc.find(fc.root, id)
	if ctl == nil {
		fc.t.Fatalf("patch for unknown control %d", id)
	}
	p := msg.Body["patch"].([]any)

	paths := map[int][]any{}
	var build func(node []any, path []any)
	build = func(node []any, path []any) {
		paths[node[0].(int)] = path
		if len(node) > 1 {
			for k, child := range node[1].(map[string]any) {
				build(child.([]any), append(slices.Clone(path), k))
			}
		}
	}
	build(p[0].([]any), nil)

	target := func(n int) any {
		var obj any = ctl
		for _, key := range paths[n] {
			switch o := obj.(type) {
			case map[string]any:
				obj = o[key.(string)]
			case *[]any:
				obj = (*o)[key.(int)]
			}
		}
		return obj
	}

	for _, raw := range p[1:] {
		op := raw.([]any)
		switch op[0].(int) {
		case protocol.OpReplace:
			target(op[1].(int)).(map[string]any)[op[2].(string)] = toClient(op[3])
		case protocol.OpAdd:
			l := target(op[1].(int)).(*[]any)
			*l = slices.Insert(*l, op[2].(int), toClient(op[3]))
		case protocol.OpRemove:
			l := target(op[1].(int)).(*[]any)
			i := op[2].(int)
			*l = slices.Delete(*l, i, i+1)
		case protocol.OpMove:
			src := target(op[1].(int)).(*[]any)
			dst := target(op[3].(int)).(*[]any)
			i := op[2].(int)
			item := (*src)[i]
			*src = slices.Delete(*src, i, i+1)
			*dst = slices.Insert(*dst, op[4].(int), item)
		}
	}
}

func newTestPage(t *testing.T) (*Page, *fakeClient) {
	fc := &fakeClient{t: t}
	p := newPage(fc.apply)
	return p, fc
}

func connect(p *Page, fc *fakeClient) {
	fc.root = toClient(p.initial()).(map[string]any)
}

func assertInSync(t *testing.T, p *Page, fc *fakeClient) {
	t.Helper()
	want := encode(p.root)
	got := fromClient(fc.root)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("client out of sync\n got: %v\nwant: %v", got, want)
	}
}

func TestUpdateSendsOnlyChangedProps(t *testing.T) {
	p, fc := newTestPage(t)
	txt := &Text{Value: "0"}
	p.Add(txt)
	connect(p, fc)

	p.Update()
	if fc.msgs != 0 {
		t.Fatalf("no change should send nothing, sent %d", fc.msgs)
	}

	txt.Value = "1"
	p.Update()
	if fc.msgs != 1 {
		t.Fatalf("expected 1 message, got %d", fc.msgs)
	}
	assertInSync(t, p, fc)
}

func TestListOpsAndPrune(t *testing.T) {
	p, fc := newTestPage(t)
	a, b, c := &Text{Value: "a"}, &Text{Value: "b"}, &Text{Value: "c"}
	col := &Column{Controls: []Control{a, b, c}}
	p.Add(col)
	connect(p, fc)

	col.Controls = []Control{c, a} // remove b, move c to the front
	p.Update()
	assertInSync(t, p, fc)
	if p.lookup(b.ID()) != nil {
		t.Error("removed control is still in the index")
	}

	p.Remove(col)
	assertInSync(t, p, fc)
	if p.lookup(a.ID()) != nil || p.lookup(col.ID()) != nil {
		t.Error("removed subtree is still in the index")
	}
}

func TestMoveBetweenParents(t *testing.T) {
	p, fc := newTestPage(t)
	x := &Text{Value: "x"}
	left := &Column{Controls: []Control{x}}
	right := &Column{}
	p.Add(&Row{Controls: []Control{left, right}})
	connect(p, fc)

	left.Controls = nil
	right.Controls = []Control{x}
	p.Update()
	assertInSync(t, p, fc)
	if p.lookup(x.ID()) == nil {
		t.Error("moved control was dropped from the index")
	}
}

func TestRandomListChanges(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	p, fc := newTestPage(t)
	col := &Column{}
	p.Add(col)
	connect(p, fc)

	n := 0
	for step := 0; step < 500; step++ {
		switch r.Intn(4) {
		case 0: // insert
			n++
			i := r.Intn(len(col.Controls) + 1)
			col.Controls = slices.Insert(col.Controls, i, Control(&Text{Value: strconv.Itoa(n)}))
		case 1: // delete
			if len(col.Controls) > 0 {
				i := r.Intn(len(col.Controls))
				col.Controls = slices.Delete(col.Controls, i, i+1)
			}
		case 2: // shuffle
			r.Shuffle(len(col.Controls), func(i, j int) {
				col.Controls[i], col.Controls[j] = col.Controls[j], col.Controls[i]
			})
		case 3: // change a value
			if len(col.Controls) > 0 {
				col.Controls[r.Intn(len(col.Controls))].(*Text).Value += "!"
			}
		}
		p.Update()
		assertInSync(t, p, fc)
	}

	// The index must hold exactly the controls in the tree (page, window,
	// overlay, dialogs, services, view, column, texts).
	if want := 7 + len(col.Controls); len(p.index) != want {
		t.Errorf("index has %d entries, want %d", len(p.index), want)
	}
}

func TestSlots(t *testing.T) {
	p, fc := newTestPage(t)
	clicks := 0
	inner := &Button{Content: "inner", OnClick: func(*Event) { clicks++ }}
	box := &Container{Content: inner}
	btn := &Button{Content: "text"}
	p.Add(box, btn)
	connect(p, fc)

	// Events reach a control nested in a single-child slot.
	p.lookup(inner.ID()).handleEvent(&Event{Name: "click"})
	if clicks != 1 {
		t.Fatalf("nested click not delivered")
	}

	// Change the nested control's props, then swap the child.
	inner.Content = "changed"
	p.Update()
	assertInSync(t, p, fc)

	box.Content = &Text{Value: "new child"}
	p.Update()
	assertInSync(t, p, fc)
	if p.lookup(inner.ID()) != nil {
		t.Error("replaced child is still in the index")
	}

	// A string-or-control field switching both ways.
	btn.Content = &Text{Value: "as control"}
	p.Update()
	assertInSync(t, p, fc)
	btn.Content = "as text again"
	p.Update()
	assertInSync(t, p, fc)

	box.Content = nil
	p.Update()
	assertInSync(t, p, fc)
}

func TestClientProps(t *testing.T) {
	p, fc := newTestPage(t)
	tf := &TextField{}
	p.Add(tf)
	connect(p, fc)

	p.applyClientProps(tf.ID(), map[string]any{"value": "typed"})
	if tf.Value != "typed" {
		t.Fatalf("Value = %q, want %q", tf.Value, "typed")
	}
	// The client already has the value: nothing is echoed back.
	p.Update()
	if fc.msgs != 0 {
		t.Fatalf("client change was echoed back (%d messages)", fc.msgs)
	}
}
