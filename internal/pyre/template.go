package pyre

import (
	"fmt"
	"strconv"
)

type tmplPart struct {
	lit   []rune
	group int // -1: literal
}

// Template is a parsed Python re.sub replacement string ("\\g<1> gen", "\\1", "\\\\", "\\n").
type Template struct {
	parts []tmplPart
}

// ParseTemplate parses a replacement string the way Python's re._parser.parse_template does, for a pattern with
// ngroup groups. Groups that did not take part in a match are replaced with "".
func ParseTemplate(repl string, ngroup int) (*Template, error) {
	src := []rune(repl)
	t := &Template{}
	var lit []rune
	addGroup := func(g int) error {
		if g > ngroup {
			return fmt.Errorf("pyre: invalid group reference %d in %q", g, repl)
		}
		if len(lit) > 0 {
			t.parts = append(t.parts, tmplPart{lit: lit, group: -1})
			lit = nil
		}
		t.parts = append(t.parts, tmplPart{group: g})
		return nil
	}
	isOct := func(r rune) bool { return r >= '0' && r <= '7' }
	isDig := func(r rune) bool { return r >= '0' && r <= '9' }
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c != '\\' {
			lit = append(lit, c)
			continue
		}
		if i+1 >= len(src) {
			return nil, fmt.Errorf("pyre: bad escape (end of pattern) in %q", repl)
		}
		i++
		c = src[i]
		switch {
		case c == 'g':
			if i+1 >= len(src) || src[i+1] != '<' {
				return nil, fmt.Errorf("pyre: missing < in %q", repl)
			}
			j := i + 2
			for j < len(src) && src[j] != '>' {
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("pyre: missing >, unterminated name in %q", repl)
			}
			name := string(src[i+2 : j])
			g, err := strconv.Atoi(name)
			if err != nil || g < 0 || !isASCII(name) || name[0] == '+' || name[0] == '-' {
				return nil, fmt.Errorf("pyre: named groups are not supported in %q", repl)
			}
			if err := addGroup(g); err != nil {
				return nil, err
			}
			i = j
		case c == '0':
			v := rune(0)
			for k := 0; k < 2 && i+1 < len(src) && isOct(src[i+1]); k++ {
				i++
				v = v*8 + src[i] - '0'
			}
			lit = append(lit, v&0xff)
		case isDig(c):
			g := int(c - '0')
			if i+1 < len(src) && isDig(src[i+1]) {
				d2 := src[i+1]
				if isOct(c) && isOct(d2) && i+2 < len(src) && isOct(src[i+2]) {
					v := (c-'0')*64 + (d2-'0')*8 + src[i+2] - '0'
					if v > 0o377 {
						return nil, fmt.Errorf("pyre: octal escape outside of range 0-0o377 in %q", repl)
					}
					lit = append(lit, v)
					i += 2
					continue
				}
				g = g*10 + int(d2-'0')
				i++
			}
			if err := addGroup(g); err != nil {
				return nil, err
			}
		default:
			switch c {
			case 'a':
				lit = append(lit, 0x07)
			case 'b':
				lit = append(lit, 0x08)
			case 'f':
				lit = append(lit, 0x0C)
			case 'n':
				lit = append(lit, 0x0A)
			case 'r':
				lit = append(lit, 0x0D)
			case 't':
				lit = append(lit, 0x09)
			case 'v':
				lit = append(lit, 0x0B)
			case '\\':
				lit = append(lit, '\\')
			default:
				if c < 0x80 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
					return nil, fmt.Errorf("pyre: bad escape \\%c in %q", c, repl)
				}
				lit = append(lit, '\\', c)
			}
		}
	}
	if len(lit) > 0 {
		t.parts = append(t.parts, tmplPart{lit: lit, group: -1})
	}
	return t, nil
}
