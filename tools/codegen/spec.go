package main

import (
	"encoding/json"
	"os"
	"strings"
)

// Spec is the JSON produced by tools/specgen/extract.py.
type Spec struct {
	FletVersion string            `json:"flet_version"`
	Classes     []*Class          `json:"classes"`
	Aliases     map[string]string `json:"aliases"`
}

// Class is a Python class from the Flet SDK.
type Class struct {
	Name     string    `json:"name"`
	Module   string    `json:"module"`
	Bases    []string  `json:"bases"`
	Doc      string    `json:"doc"`
	Kind     string    `json:"kind"` // control, value, enum, class
	DartName string    `json:"dart_name"`
	Fields   []*Field  `json:"fields"`
	Members  []*Member `json:"members"`
	Methods  []*Method `json:"methods"`
	// PlainDataclass values send their defaults too (see extract.py).
	PlainDataclass bool `json:"plain_dataclass"`
}

// Field is an annotated class attribute.
type Field struct {
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Default        *string        `json:"default"`
	DefaultFactory *string        `json:"default_factory"`
	Metadata       map[string]any `json:"metadata"`
	Skip           bool           `json:"skip"`
	Init           *bool          `json:"init"`
	Doc            string         `json:"doc"`
}

// Member is an enum member.
type Member struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
	Doc   string `json:"doc"`
}

func loadSpec(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// hasDefault reports whether the field has a default value
// (fields without one are required in Python).
func (f *Field) hasDefault() bool {
	return f.Default != nil || f.DefaultFactory != nil
}

func (f *Field) defaultValue() string {
	if f.Default == nil {
		return ""
	}
	return strings.TrimSpace(*f.Default)
}

// wireName is the key used on the wire (data_field metadata, if any).
func (f *Field) wireName() string {
	if v, ok := f.Metadata["data_field"].(string); ok && v != "" {
		return v
	}
	return f.Name
}
