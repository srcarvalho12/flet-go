package main

import (
	"fmt"
	"go/token"
	"path/filepath"
	"strings"
)

// Method is an async Python method that calls a client method
// (see parse_method in tools/specgen/extract.py).
type Method struct {
	Name          string            `json:"name"`
	Params        []*Param          `json:"params"`
	Returns       *string           `json:"returns"`
	Doc           string            `json:"doc"`
	Simple        bool              `json:"simple"`
	ReturnsResult bool              `json:"returns_result"`
	Invoke        string            `json:"invoke"`
	Args          map[string]string `json:"args"`
	Timeout       *string           `json:"timeout"`
}

// Param is a method parameter.
type Param struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Default *string `json:"default"`
}

// method is a method ready to be emitted for a control.
type method struct {
	*Method
	goName   string
	declIn   string
	required []*field // positional parameters
	optional []*field // fields of the options struct
	argKey   map[string]string
	result   string // Go result type, "" when the method returns nothing
	options  string // options struct name, "" when there are no optional parameters
}

// methods returns the client methods available on a control, following the
// Python method resolution order. Methods that cannot be generated are counted
// in the report.
func (m *model) methods(name string, fieldNames map[string]bool) []*method {
	var out []*method
	seen := map[string]bool{}
	for _, c := range m.linearize(name) {
		if c == nil || reserved[c.Name] && c.Name != "Window" {
			continue
		}
		for _, pm := range c.Methods {
			if seen[pm.Name] {
				continue
			}
			seen[pm.Name] = true
			mt, why := m.buildMethod(c.Name, pm)
			if why != "" {
				m.report["method: "+why]++
				continue
			}
			if fieldNames[mt.goName] || mt.goName == "ID" {
				m.report["method: name clashes with a field"]++
				continue
			}
			out = append(out, mt)
		}
	}
	return out
}

func (m *model) buildMethod(declIn string, pm *Method) (*method, string) {
	if !pm.Simple {
		return nil, "not a plain client call"
	}
	mt := &method{Method: pm, goName: goName(pm.Name), declIn: declIn, argKey: map[string]string{}}

	// Each argument must be a parameter passed through unchanged.
	params := map[string]*Param{}
	for _, p := range pm.Params {
		params[p.Name] = p
	}
	for key, expr := range pm.Args {
		if params[expr] == nil {
			return nil, "computed argument"
		}
		mt.argKey[expr] = key
	}

	for _, p := range pm.Params {
		if _, used := mt.argKey[p.Name]; !used {
			return nil, "unused parameter"
		}
		f := &field{Field: &Field{Name: p.Name, Type: p.Type, Default: p.Default}}
		f.goName = goName(p.Name)
		f.typ = m.resolveField(f.Field)
		if f.typ.skip != "" {
			return nil, "parameter type: " + f.typ.skip
		}
		switch f.typ.kind {
		case kControl, kControls, kStrOrControl, kIconOrCtrl, kHandler:
			return nil, "parameter type: " + f.typ.kind
		}
		if !f.hasDefault() {
			f.required = true
			mt.required = append(mt.required, f)
			continue
		}
		f.pointer = f.typ.kind == kScalar && !isZeroDefault(f.defaultValue())
		if f.pointer {
			f.typ.goTyp = "*" + f.typ.goTyp
		}
		mt.optional = append(mt.optional, f)
	}

	if pm.ReturnsResult && pm.Returns != nil && *pm.Returns != "None" {
		t, err := parseType(*pm.Returns)
		if err != nil {
			return nil, "unparsable return type"
		}
		mt.result = m.resultType(t)
	}
	return mt, ""
}

// resultType maps a Python return annotation to the Go type the client
// result is decoded into.
func (m *model) resultType(t *texpr) string {
	switch t.name {
	case "Optional":
		return m.resultType(t.args[0])
	case "str":
		return "string"
	case "bool":
		return "bool"
	case "int":
		return "int"
	case "float", "Number":
		return "float64"
	case "bytes":
		return "[]byte"
	case "dict", "Dict":
		return "map[string]any"
	case "list", "List":
		if len(t.args) == 1 {
			if e := m.resultType(t.args[0]); e != "any" {
				return "[]" + e
			}
		}
		return "[]any"
	}
	if m.enums[t.name] != nil {
		return t.name
	}
	if m.values[t.name] != nil {
		return "*" + t.name
	}
	return "any"
}

// assignOptionNames gives each options struct a unique name: MethodOptions,
// or DeclaringClassMethodOptions when several classes share a method name.
func (m *model) assignOptionNames(all map[string][]*method) {
	decls := map[string]map[string]bool{} // Go method name -> declaring classes
	for _, ms := range all {
		for _, mt := range ms {
			if len(mt.optional) == 0 {
				continue
			}
			if decls[mt.goName] == nil {
				decls[mt.goName] = map[string]bool{}
			}
			decls[mt.goName][mt.declIn] = true
		}
	}
	for _, ms := range all {
		for _, mt := range ms {
			if len(mt.optional) == 0 {
				continue
			}
			mt.options = mt.goName + "Options"
			if len(decls[mt.goName]) > 1 || m.classes[mt.options] != nil {
				mt.options = mt.declIn + mt.goName + "Options"
			}
		}
	}
}

// paramName converts a Python parameter name to a Go identifier.
func paramName(s string) string {
	parts := strings.Split(s, "_")
	out := strings.ToLower(parts[0])
	for _, p := range parts[1:] {
		out += goName(p)
	}
	if token.IsKeyword(out) {
		out += "_"
	}
	return out
}

// emitMethods writes the methods of every control and their options structs.
func (m *model) emitMethods(out string, all map[string][]*method) error {
	w := &writer{}
	w.p(header+"package flet\n\nimport (\n\"maps\"\n\"slices\"\n\"time\"\n)\n\nvar _ = maps.Clone[map[string]any]\nvar _ = slices.Clone[[]string]\nvar _ time.Duration", m.spec.FletVersion)

	emitted := map[string]bool{}
	for _, name := range sortedNames(all) {
		for _, mt := range all[name] {
			if mt.options == "" || emitted[mt.options] {
				continue
			}
			emitted[mt.options] = true
			w.p("// %s are the optional arguments of %s.%s.", mt.options, mt.declIn, mt.goName)
			w.p("type %s struct {", mt.options)
			for _, f := range mt.optional {
				fieldDecl(w, f, false)
			}
			w.p("}")
			w.p("")
		}
	}

	for _, name := range sortedNames(all) {
		for _, mt := range all[name] {
			var params []string
			for _, f := range mt.required {
				params = append(params, paramName(f.Name)+" "+f.typ.goTyp)
			}
			if mt.options != "" {
				params = append(params, "opts ..."+mt.options)
			}
			ret := "error"
			if mt.result != "" {
				ret = fmt.Sprintf("(%s, error)", mt.result)
			}
			w.doc("", mt.goName+" ", lowerFirst(mt.Doc))
			w.p("func (c *%s) %s(%s) %s {", name, mt.goName, strings.Join(params, ", "), ret)
			args := "nil"
			if len(mt.required)+len(mt.optional) > 0 {
				args = "m"
				w.p("m := map[string]any{}")
				for _, f := range mt.required {
					w.p("m[%q] = encodeArg(%s)", mt.argKey[f.Name], paramName(f.Name))
				}
				if mt.options != "" {
					w.p("if len(opts) > 0 {")
					w.p("o := opts[0]")
					for _, f := range mt.optional {
						emitEncode(w, "o", f, mt.argKey[f.Name])
					}
					w.p("}")
				}
			}
			if mt.result == "" {
				w.p("_, err := c.invoke(%q, %s)", mt.Invoke, args)
				w.p("return err")
			} else {
				w.p("return invokeResult[%s](&c.controlBase, %q, %s)", mt.result, mt.Invoke, args)
			}
			w.p("}")
			w.p("")
		}
	}
	return writeGo(filepath.Join(out, "methods_gen.go"), w)
}

// lowerFirst turns "Retrieves data." into "retrieves data." so the comment
// reads "Get retrieves data.".
func lowerFirst(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1 && s[0] >= 'A' && s[0] <= 'Z' && !(s[1] >= 'A' && s[1] <= 'Z') {
		return strings.ToLower(s[:1]) + s[1:]
	}
	return s
}
