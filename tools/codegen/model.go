package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Kinds of resolved field types.
const (
	kScalar       = "scalar"       // string, bool, int, float64
	kEnum         = "enum"         // generated enum type
	kColor        = "color"        // Color
	kIcon         = "icon"         // IconData
	kDuration     = "duration"     // time.Duration, sent as milliseconds
	kValue        = "value"        // *ValueType
	kSlice        = "slice"        // []T of scalars/enums/colors
	kValueSlice   = "valueslice"   // []*ValueType
	kMap          = "map"          // map[string]any
	kAny          = "any"          // union of several types
	kControl      = "control"      // single child control
	kControls     = "controls"     // list of child controls
	kStrOrControl = "strorcontrol" // string or child control
	kIconOrCtrl   = "iconorcontrol"
	kHandler      = "handler" // event handler
)

// gtype is a resolved Go type for a Python annotation.
type gtype struct {
	kind  string
	goTyp string // Go type, e.g. "float64", "*TextStyle", "[]string"
	elem  string // element type for slices
	event string // event struct for handlers ("" = Event)
	skip  string // non-empty: field is not supported, with the reason
}

type model struct {
	spec    *Spec
	classes map[string]*Class
	aliases map[string]string
	report  map[string]int // skipped field reasons → count

	enums    map[string]*Class
	values   map[string]*Class
	events   map[string]*Class
	controls map[string]*Class
}

// Names handled by hand-written code or not generated yet.
var reserved = map[string]bool{
	"Event": true, "Control": true, "BaseControl": true, "Page": true, "BasePage": true,
	"Service": true, "ServiceRegistry": true, "MultiView": true,
	"IconData": true, "Colors": true, "CupertinoColors": true, "Duration": true,
	"Ref": true, "KeyValue": true, "ValueKey": true, "ScrollKey": true,
}

func newModel(s *Spec) *model {
	m := &model{
		spec:     s,
		classes:  map[string]*Class{},
		aliases:  s.Aliases,
		report:   map[string]int{},
		enums:    map[string]*Class{},
		values:   map[string]*Class{},
		events:   map[string]*Class{},
		controls: map[string]*Class{},
	}
	for _, c := range s.Classes {
		// Canvas shapes reuse names like Text and Image; not generated yet.
		if strings.Contains(c.Module, ".canvas") {
			continue
		}
		if _, dup := m.classes[c.Name]; dup {
			m.report["duplicate class "+c.Name]++
			continue
		}
		m.classes[c.Name] = c
	}
	for name, c := range m.classes {
		switch {
		case reserved[name]:
		case c.Kind == "enum":
			if enumValueKind(c) != "" {
				m.enums[name] = c
			}
		case m.isA(name, "BaseControl"):
			if c.DartName != "" {
				m.controls[name] = c
			}
		case m.isA(name, "Event"):
			m.events[name] = c
		case c.Kind == "value":
			m.values[name] = c
		}
	}
	return m
}

// isA reports whether class name is base or inherits from it.
func (m *model) isA(name, base string) bool {
	seen := map[string]bool{}
	var walk func(n string) bool
	walk = func(n string) bool {
		if n == base {
			return true
		}
		if seen[n] {
			return false
		}
		seen[n] = true
		c := m.classes[n]
		if c == nil {
			return false
		}
		for _, b := range c.Bases {
			if walk(b) {
				return true
			}
		}
		return false
	}
	return walk(name)
}

// linearize returns the class and its bases in Python MRO order (C3).
func (m *model) linearize(name string) []*Class {
	var lin func(n string) []string
	lin = func(n string) []string {
		c := m.classes[n]
		if c == nil {
			return nil
		}
		var seqs [][]string
		for _, b := range c.Bases {
			if l := lin(b); len(l) > 0 {
				seqs = append(seqs, l)
			}
		}
		var bases []string
		for _, b := range c.Bases {
			if m.classes[b] != nil {
				bases = append(bases, b)
			}
		}
		if len(bases) > 0 {
			seqs = append(seqs, bases)
		}
		out := []string{n}
		for {
			var nonEmpty [][]string
			for _, s := range seqs {
				if len(s) > 0 {
					nonEmpty = append(nonEmpty, s)
				}
			}
			if len(nonEmpty) == 0 {
				return out
			}
			seqs = nonEmpty
			var head string
			for _, s := range seqs {
				cand := s[0]
				inTail := false
				for _, t := range seqs {
					for _, x := range t[1:] {
						if x == cand {
							inTail = true
						}
					}
				}
				if !inTail {
					head = cand
					break
				}
			}
			if head == "" {
				return out // inconsistent hierarchy; keep what we have
			}
			out = append(out, head)
			for i, s := range seqs {
				if s[0] == head {
					seqs[i] = s[1:]
				}
			}
		}
	}
	var out []*Class
	for _, n := range lin(name) {
		out = append(out, m.classes[n])
	}
	return out
}

// field is a flattened field of a generated struct.
type field struct {
	*Field
	goName      string
	typ         gtype
	pointer     bool   // scalar stored as a pointer (default is not the zero value)
	required    bool   // no default in Python: always sent
	override    string // Go literal to send when unset (subclass overrides the default)
	declaringIn string
}

// fields returns the flattened fields of a class, base fields first.
func (m *model) fields(name string, stopAt map[string]bool) []*field {
	lin := m.linearize(name)
	byName := map[string]*field{}
	var order []string
	rootDefault := map[string]*Field{}
	// Walk from the root so derived classes override base definitions.
	for i := len(lin) - 1; i >= 0; i-- {
		c := lin[i]
		if stopAt[c.Name] {
			continue
		}
		for _, f := range c.Fields {
			if f.Skip || strings.HasPrefix(f.Name, "_") || (f.Init != nil && !*f.Init) {
				continue
			}
			if c.Name == "BaseControl" || (f.Name == "data" || f.Name == "key") && m.isA(c.Name, "BaseControl") {
				continue
			}
			if _, ok := rootDefault[f.Name]; !ok {
				rootDefault[f.Name] = f
			}
			if _, ok := byName[f.Name]; !ok {
				order = append(order, f.Name)
			}
			byName[f.Name] = &field{Field: f, declaringIn: c.Name}
		}
	}
	var out []*field
	for _, n := range order {
		f := byName[n]
		f.goName = goName(f.Name)
		f.typ = m.resolveField(f.Field)
		if f.typ.skip != "" {
			m.report[f.typ.skip]++
			continue
		}
		f.required = !f.hasDefault()
		f.pointer = f.typ.kind == kScalar && f.hasDefault() && !isZeroDefault(f.defaultValue())
		if root := rootDefault[n]; root != f.Field && root.defaultValue() != f.defaultValue() {
			f.override = m.literal(f, f.defaultValue())
			if f.override == "" && f.defaultValue() != "None" {
				m.report["unsupported default override"]++
			}
		}
		// Plain dataclasses always send fields with a non-None default, so
		// an unset Go field sends the Python default.
		if c := m.classes[name]; c != nil && c.PlainDataclass && f.override == "" &&
			f.hasDefault() && f.defaultValue() != "None" && f.Default != nil {
			f.override = m.literal(f, f.defaultValue())
		}
		if f.pointer {
			f.typ.goTyp = "*" + f.typ.goTyp
		}
		out = append(out, f)
	}
	return out
}

func isZeroDefault(d string) bool {
	switch d {
	case "", "None", "False", "0", "0.0", "''", `""`:
		return true
	}
	if strings.Contains(d, ".") && !strings.ContainsAny(d, "0123456789") {
		return true // enum member: unset means the client default
	}
	if f, err := strconv.ParseFloat(d, 64); err == nil {
		return f == 0
	}
	return false
}

// literal converts a Python default into a Go literal of the field's type.
func (m *model) literal(f *field, d string) string {
	switch f.typ.kind {
	case kScalar:
		switch d {
		case "True":
			return "true"
		case "False":
			return "false"
		}
		if _, err := strconv.ParseFloat(d, 64); err == nil {
			return d
		}
		if s, err := strconv.Unquote(strings.ReplaceAll(d, "'", `"`)); err == nil {
			return strconv.Quote(s)
		}
	case kEnum:
		parts := strings.Split(d, ".")
		if len(parts) == 2 {
			if c := m.enums[parts[0]]; c != nil {
				for _, mem := range c.Members {
					if mem.Name == parts[1] {
						return enumConst(c.Name, mem.Name)
					}
				}
			}
		}
	}
	return ""
}

func (m *model) resolveField(f *Field) gtype {
	t, err := parseType(f.Type)
	if err != nil {
		return gtype{skip: "unparsable type"}
	}
	return m.resolve(t, 0)
}

func (m *model) resolve(t *texpr, depth int) gtype {
	if depth > 10 {
		return gtype{skip: "alias recursion"}
	}
	switch t.name {
	case "Optional", "Annotated", "Required", "NotRequired":
		return m.resolve(t.args[0], depth+1)
	case "Union":
		return m.resolveUnion(t, depth)
	case "list", "List", "Sequence":
		if len(t.args) != 1 {
			return gtype{skip: "list without element type"}
		}
		e := m.resolve(t.args[0], depth+1)
		switch e.kind {
		case kControl:
			return gtype{kind: kControls, goTyp: "[]Control"}
		case kScalar, kEnum, kColor:
			return gtype{kind: kSlice, goTyp: "[]" + e.goTyp, elem: e.goTyp}
		case kValue:
			return gtype{kind: kValueSlice, goTyp: "[]" + e.goTyp, elem: e.goTyp}
		}
		return gtype{skip: "list of " + e.kind + e.skip}
	case "dict", "Dict":
		if len(t.args) == 2 && t.args[0].name == "str" && t.args[1].name == "str" {
			return gtype{kind: kMap, goTyp: "map[string]string"}
		}
		return gtype{kind: kMap, goTyp: "map[string]any"}
	case "ControlEventHandler":
		return gtype{kind: kHandler, goTyp: "func(e *Event)"}
	case "EventHandler":
		if len(t.args) == 1 {
			if ev := t.args[0].name; m.events[ev] != nil {
				return gtype{kind: kHandler, goTyp: "func(e *" + ev + ")", event: ev}
			}
		}
		return gtype{kind: kHandler, goTyp: "func(e *Event)"}
	case "ControlStateValue":
		return gtype{kind: kAny, goTyp: "any"}
	case "Callable":
		return gtype{skip: "callable"}
	case "str":
		return gtype{kind: kScalar, goTyp: "string"}
	case "bytes":
		return gtype{kind: kSlice, goTyp: "[]byte", elem: "byte"}
	case "bool":
		return gtype{kind: kScalar, goTyp: "bool"}
	case "int":
		return gtype{kind: kScalar, goTyp: "int"}
	case "float", "Number":
		return gtype{kind: kScalar, goTyp: "float64"}
	case "Any", "object":
		return gtype{kind: kAny, goTyp: "any"}
	case "ColorValue", "Colors", "CupertinoColors":
		return gtype{kind: kColor, goTyp: "Color"}
	case "IconData", "Icons", "CupertinoIcons":
		return gtype{kind: kIcon, goTyp: "IconData"}
	case "DurationValue", "Duration":
		return gtype{kind: kDuration, goTyp: "time.Duration"}
	case "Control", "BaseControl":
		return gtype{kind: kControl, goTyp: "Control"}
	}
	if m.controls[t.name] != nil || m.classes[t.name] != nil && m.isA(t.name, "Control") {
		return gtype{kind: kControl, goTyp: "Control"}
	}
	if m.enums[t.name] != nil {
		return gtype{kind: kEnum, goTyp: t.name}
	}
	if m.values[t.name] != nil {
		return gtype{kind: kValue, goTyp: "*" + t.name}
	}
	if a, ok := m.aliases[t.name]; ok && a != "TypeVar" {
		at, err := parseType(a)
		if err != nil {
			return gtype{skip: "unparsable alias " + t.name}
		}
		return m.resolve(at, depth+1)
	}
	return gtype{skip: "unknown type " + t.name}
}

func (m *model) resolveUnion(t *texpr, depth int) gtype {
	var members []gtype
	hasStr, hasControl, hasIcon := false, false, false
	for _, a := range t.args {
		if a.name == "None" {
			continue
		}
		g := m.resolve(a, depth+1)
		if g.skip != "" {
			continue
		}
		members = append(members, g)
		switch {
		case g.kind == kScalar && g.goTyp == "string":
			hasStr = true
		case g.kind == kControl:
			hasControl = true
		case g.kind == kIcon:
			hasIcon = true
		}
	}
	if len(members) == 0 {
		return gtype{skip: "union of unsupported types"}
	}
	if len(members) == 1 {
		return members[0]
	}
	allControls := true
	for _, g := range members {
		if g.kind != kControl {
			allControls = false
		}
	}
	switch {
	case allControls:
		return gtype{kind: kControl, goTyp: "Control"}
	case hasControl && hasStr && len(members) == 2:
		return gtype{kind: kStrOrControl, goTyp: "any"}
	case hasControl && hasIcon && len(members) == 2:
		return gtype{kind: kIconOrCtrl, goTyp: "any"}
	case hasControl:
		return gtype{skip: "union with control"}
	}
	// Numbers: int | float.
	if len(members) == 2 && members[0].goTyp == "int" && members[1].goTyp == "float64" {
		return members[1]
	}
	return gtype{kind: kAny, goTyp: "any"}
}

func enumValueKind(c *Class) string {
	kind := ""
	for _, mem := range c.Members {
		k := ""
		switch mem.Value.(type) {
		case string:
			k = "string"
		case float64:
			k = "int"
		default:
			return ""
		}
		if kind != "" && kind != k {
			return ""
		}
		kind = k
	}
	return kind
}

func enumConst(enum, member string) string {
	return enum + goName(strings.ToLower(member))
}

var initialisms = map[string]string{
	"id": "ID", "url": "URL", "uri": "URI", "html": "HTML", "json": "JSON",
	"api": "API", "svg": "SVG", "http": "HTTP", "https": "HTTPS", "ip": "IP", "ui": "UI",
	"appbar": "AppBar",
}

// goName converts snake_case to CamelCase.
func goName(s string) string {
	var b strings.Builder
	for _, p := range strings.Split(s, "_") {
		if p == "" {
			continue
		}
		if v, ok := initialisms[strings.ToLower(p)]; ok {
			b.WriteString(v)
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	out := b.String()
	if out == "" || out[0] >= '0' && out[0] <= '9' {
		out = "X" + out
	}
	return out
}

func (m *model) printReport() {
	keys := make([]string, 0, len(m.report))
	for k := range m.report {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m.report[keys[i]] > m.report[keys[j]] })
	fmt.Printf("generated: %d controls, %d value types, %d events, %d enums\n",
		len(m.controls), len(m.values), len(m.events), len(m.enums))
	fmt.Println("skipped:")
	for _, k := range keys {
		fmt.Printf("  %4d  %s\n", m.report[k], k)
	}
}
