package flet

import (
	"reflect"
	"strconv"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

//go:generate go run ./tools/codegen -spec spec/flet.json -out .

// Color is a color name (see the colors package), a hex value such as
// "#ff0000", or a name with opacity such as "red,0.5".
type Color string

// WithOpacity returns the color with the given opacity (0.0 to 1.0).
func (c Color) WithOpacity(opacity float64) Color {
	return Color(string(c) + "," + strconv.FormatFloat(opacity, 'f', -1, 64))
}

// IconData is an icon code point (see the icons package).
type IconData int

// Bool returns a pointer to v, for optional fields whose default is not false.
func Bool(v bool) *bool { return &v }

// Float returns a pointer to v, for optional fields whose default is not zero.
func Float(v float64) *float64 { return &v }

// Int returns a pointer to v, for optional fields whose default is not zero.
func Int(v int) *int { return &v }

// Str returns a pointer to v, for optional fields whose default is not empty.
func Str(v string) *string { return &v }

// valueType is implemented by the generated value types (TextStyle, Padding...).
type valueType interface {
	toMap() map[string]any
}

// encodeAny encodes a value stored in a field that accepts several types.
func encodeAny(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case valueType:
		return x.toMap()
	case time.Duration:
		return x.Milliseconds()
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = encodeAny(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = encodeAny(e)
		}
		return out
	}
	return v
}

// propsSetter is implemented by controls whose properties the client can
// change, such as TextField.value.
type propsSetter interface {
	applyProps(p map[string]any)
}

// convert converts a decoded MessagePack value to T (numbers to numbers,
// strings to named string types such as Color).
func convert[T any](v any) (T, bool) {
	var zero T
	if v == nil {
		return zero, true
	}
	rv := reflect.ValueOf(v)
	t := reflect.TypeOf(zero)
	isStr := func(k reflect.Kind) bool { return k == reflect.String }
	if isStr(rv.Kind()) != isStr(t.Kind()) || !rv.CanConvert(t) {
		return zero, false
	}
	return rv.Convert(t).Interface().(T), true
}

func assign[T any](dst *T, v any) {
	if x, ok := convert[T](v); ok {
		*dst = x
	}
}

func assignPtr[T any](dst **T, v any) {
	if v == nil {
		*dst = nil
		return
	}
	if x, ok := convert[T](v); ok {
		*dst = &x
	}
}

// decodeEvent fills a typed event from the raw event data.
func decodeEvent(e *Event, target any) {
	if e.Data == nil {
		return
	}
	b, err := msgpack.Marshal(e.Data)
	if err != nil {
		return
	}
	_ = msgpack.Unmarshal(b, target)
}
