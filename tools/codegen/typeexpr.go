package main

import (
	"fmt"
	"strings"
	"unicode"
)

// texpr is a parsed Python type annotation, e.g. Optional[list[Control]].
// Unions written with "|" are normalized to Union[...]; a bracketed list
// such as the first argument of Callable[[int], Any] has name "[]".
type texpr struct {
	name string
	args []*texpr
}

func (t *texpr) String() string {
	if len(t.args) == 0 {
		return t.name
	}
	parts := make([]string, len(t.args))
	for i, a := range t.args {
		parts[i] = a.String()
	}
	return t.name + "[" + strings.Join(parts, ", ") + "]"
}

type tparser struct {
	s   string
	pos int
}

func parseType(s string) (*texpr, error) {
	p := &tparser{s: s}
	t, err := p.expr()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return nil, fmt.Errorf("unexpected %q in %q", p.s[p.pos:], s)
	}
	return t, nil
}

func (p *tparser) skipSpace() {
	for p.pos < len(p.s) && p.s[p.pos] == ' ' {
		p.pos++
	}
}

func (p *tparser) peek() byte {
	p.skipSpace()
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *tparser) expr() (*texpr, error) {
	t, err := p.term()
	if err != nil {
		return nil, err
	}
	if p.peek() != '|' {
		return t, nil
	}
	u := &texpr{name: "Union", args: []*texpr{t}}
	for p.peek() == '|' {
		p.pos++
		t, err := p.term()
		if err != nil {
			return nil, err
		}
		u.args = append(u.args, t)
	}
	return u, nil
}

func (p *tparser) term() (*texpr, error) {
	switch c := p.peek(); {
	case c == '\'' || c == '"':
		end := strings.IndexByte(p.s[p.pos+1:], c)
		if end < 0 {
			return nil, fmt.Errorf("unterminated string in %q", p.s)
		}
		inner := p.s[p.pos+1 : p.pos+1+end]
		p.pos += end + 2
		return parseType(inner)
	case c == '[':
		p.pos++
		t := &texpr{name: "[]"}
		if p.peek() == ']' {
			p.pos++
			return t, nil
		}
		args, err := p.list(']')
		t.args = args
		return t, err
	case c == '.' && strings.HasPrefix(p.s[p.pos:], "..."):
		p.pos += 3
		return &texpr{name: "..."}, nil
	}

	start := p.pos
	for p.pos < len(p.s) {
		r := rune(p.s[p.pos])
		if r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			p.pos++
			continue
		}
		break
	}
	if start == p.pos {
		return nil, fmt.Errorf("expected a name at %d in %q", p.pos, p.s)
	}
	name := p.s[start:p.pos]
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[i+1:]
	}
	t := &texpr{name: name}
	if p.peek() == '[' {
		p.pos++
		args, err := p.list(']')
		if err != nil {
			return nil, err
		}
		t.args = args
	} else if p.peek() == '(' {
		// Call inside Annotated metadata, e.g. V.between(0.0, 1.0): skip it.
		depth := 0
		for p.pos < len(p.s) {
			switch p.s[p.pos] {
			case '(':
				depth++
			case ')':
				depth--
			}
			p.pos++
			if depth == 0 {
				break
			}
		}
	}
	return t, nil
}

func (p *tparser) list(close byte) ([]*texpr, error) {
	var args []*texpr
	for {
		t, err := p.expr()
		if err != nil {
			return nil, err
		}
		args = append(args, t)
		switch p.peek() {
		case ',':
			p.pos++
		case close:
			p.pos++
			return args, nil
		default:
			return nil, fmt.Errorf("expected ',' or %q at %d in %q", close, p.pos, p.s)
		}
	}
}
