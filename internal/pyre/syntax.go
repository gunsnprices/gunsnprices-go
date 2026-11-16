package pyre

import (
	"fmt"
	"strconv"
	"strings"
)

// The subset of Python's re syntax that the parsing rules use, read the way Python's re._parser reads it:
// literals and escapes, character classes (with \d \w \s and their negations), ".", "^", "$", \A, \Z, \b, \B,
// capturing, non-capturing and named groups, look-ahead and fixed-width look-behind, comments, alternation, and
// greedy or lazy repetition (*, +, ?, {m}, {m,}, {,n}, {m,n}; any other "{" is a literal). Backreferences,
// conditionals, inline flags, atomic groups and possessive repetition are rejected.

type kind uint8

const (
	kLit      kind = iota // one character
	kClass                // character class
	kAny                  // "." (anything but "\n")
	kBegin                // "^" and \A: start of the string
	kEnd                  // "$": end of the string or before a final "\n"
	kEndStr               // \Z: end of the string
	kWordB                // \b
	kNotWordB             // \B
	kGroup                // capturing group
	kLook                 // look-ahead or look-behind
	kConcat               // sequence
	kAlt                  // alternation
	kRepeat               // repetition
)

const inf = -1

type ast struct {
	kind     kind
	r        rune      // kLit
	cls      *classAST // kClass
	subs     []*ast    // kConcat, kAlt; one element for kGroup, kLook, kRepeat
	idx      int       // kGroup: group number
	min, max int       // kRepeat (max == inf: no limit)
	greedy   bool      // kRepeat
	neg      bool      // kLook: negative
	behind   bool      // kLook: look-behind
	width    int       // kLook (behind): width of the sub-pattern
	cm       *charMatcher
}

type classAST struct {
	neg    bool
	ranges [][2]rune // explicit members, single characters as (c, c)
	cats   []byte    // 'd', 'D', 'w', 'W', 's', 'S'
}

type parser struct {
	src    []rune
	pos    int
	ngroup int
}

// Error is a pattern that cannot be compiled.
type Error struct {
	Pattern string
	Msg     string
}

func (e *Error) Error() string { return fmt.Sprintf("pyre: %s in %q", e.Msg, e.Pattern) }

type parseError string

func parse(pattern string) (tree *ast, ngroup int, err error) {
	p := &parser{src: []rune(pattern)}
	defer func() {
		if r := recover(); r != nil {
			msg, ok := r.(parseError)
			if !ok {
				panic(r)
			}
			err = &Error{Pattern: pattern, Msg: string(msg)}
		}
	}()
	tree = p.alternation()
	if p.pos < len(p.src) {
		p.fail("unbalanced parenthesis")
	}
	return tree, p.ngroup, nil
}

func (p *parser) fail(msg string) { panic(parseError(fmt.Sprintf("%s at position %d", msg, p.pos))) }

func (p *parser) more() bool { return p.pos < len(p.src) }

func (p *parser) peek() rune {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return -1
}

func (p *parser) accept(r rune) bool {
	if p.peek() == r {
		p.pos++
		return true
	}
	return false
}

func (p *parser) alternation() *ast {
	alts := []*ast{p.sequence()}
	for p.accept('|') {
		alts = append(alts, p.sequence())
	}
	if len(alts) == 1 {
		return alts[0]
	}
	return &ast{kind: kAlt, subs: alts}
}

func (p *parser) sequence() *ast {
	seq := &ast{kind: kConcat}
	for p.more() {
		c := p.peek()
		if c == '|' || c == ')' {
			break
		}
		p.pos++
		var item *ast
		switch c {
		case '\\':
			item = p.escape()
		case '[':
			item = &ast{kind: kClass, cls: p.class()}
		case '(':
			item = p.group()
			if item == nil { // comment
				continue
			}
		case '.':
			item = &ast{kind: kAny}
		case '^':
			item = &ast{kind: kBegin}
		case '$':
			item = &ast{kind: kEnd}
		case '*', '+', '?':
			p.repeat(seq, c)
			continue
		case '{':
			if p.braces(seq) {
				continue
			}
			item = &ast{kind: kLit, r: '{'}
		default:
			item = &ast{kind: kLit, r: c}
		}
		seq.subs = append(seq.subs, item)
	}
	return seq
}

// repeat applies "*", "+" or "?" to the last item of seq.
func (p *parser) repeat(seq *ast, c rune) {
	lo, hi := 0, inf
	switch c {
	case '+':
		lo = 1
	case '?':
		hi = 1
	}
	p.applyRepeat(seq, lo, hi)
}

// braces reads "{m}", "{m,}", "{,n}", "{m,n}" after "{" and applies it; false (and nothing consumed) if the brace is
// a literal.
func (p *parser) braces(seq *ast) bool {
	start := p.pos
	if p.peek() == '}' {
		return false
	}
	digits := func() string {
		var b strings.Builder
		for p.more() && p.peek() >= '0' && p.peek() <= '9' {
			b.WriteRune(p.src[p.pos])
			p.pos++
		}
		return b.String()
	}
	lo := digits()
	hi := lo
	if p.accept(',') {
		hi = digits()
	}
	if !p.accept('}') {
		p.pos = start
		return false
	}
	min, max := 0, inf
	if lo != "" {
		v, err := strconv.Atoi(lo)
		if err != nil || v >= 1<<30 {
			p.fail("the repetition number is too large")
		}
		min = v
	}
	if hi != "" {
		v, err := strconv.Atoi(hi)
		if err != nil || v >= 1<<30 {
			p.fail("the repetition number is too large")
		}
		max = v
		if max < min {
			p.fail("min repeat greater than max repeat")
		}
	}
	p.applyRepeat(seq, min, max)
	return true
}

func (p *parser) applyRepeat(seq *ast, min, max int) {
	if len(seq.subs) == 0 {
		p.fail("nothing to repeat")
	}
	last := seq.subs[len(seq.subs)-1]
	switch last.kind {
	case kBegin, kEnd, kEndStr, kWordB, kNotWordB:
		p.fail("nothing to repeat")
	case kRepeat:
		p.fail("multiple repeat")
	}
	greedy := true
	if p.accept('?') {
		greedy = false
	} else if p.peek() == '+' {
		p.fail("possessive repetition is not supported")
	}
	seq.subs[len(seq.subs)-1] = &ast{kind: kRepeat, subs: []*ast{last}, min: min, max: max, greedy: greedy}
}

// group reads a group after "("; nil for a comment.
func (p *parser) group() *ast {
	if !p.accept('?') {
		p.ngroup++
		idx := p.ngroup
		sub := p.alternation()
		p.close()
		return &ast{kind: kGroup, idx: idx, subs: []*ast{sub}}
	}
	switch {
	case p.accept(':'):
		sub := p.alternation()
		p.close()
		return sub
	case p.accept('='), p.accept('!'):
		neg := p.src[p.pos-1] == '!'
		sub := p.alternation()
		p.close()
		return &ast{kind: kLook, neg: neg, subs: []*ast{sub}}
	case p.accept('#'):
		for p.more() && p.peek() != ')' {
			p.pos++
		}
		if !p.accept(')') {
			p.fail("missing ), unterminated comment")
		}
		return nil
	case p.accept('<'):
		if p.accept('=') || p.accept('!') {
			neg := p.src[p.pos-1] == '!'
			sub := p.alternation()
			p.close()
			lo, hi := width(sub)
			if lo != hi {
				p.fail("look-behind requires fixed-width pattern")
			}
			return &ast{kind: kLook, neg: neg, behind: true, width: lo, subs: []*ast{sub}}
		}
	case p.accept('P'):
		if p.accept('<') {
			return p.namedGroup()
		}
	}
	p.fail("unsupported group syntax")
	return nil
}

func (p *parser) namedGroup() *ast {
	for p.more() && p.peek() != '>' {
		p.pos++
	}
	if !p.accept('>') {
		p.fail("missing >, unterminated name")
	}
	p.ngroup++
	idx := p.ngroup
	sub := p.alternation()
	p.close()
	return &ast{kind: kGroup, idx: idx, subs: []*ast{sub}}
}

func (p *parser) close() {
	if !p.accept(')') {
		p.fail("missing ), unterminated subpattern")
	}
}

// escape reads an escape after "\" outside a class.
func (p *parser) escape() *ast {
	if !p.more() {
		p.fail("bad escape (end of pattern)")
	}
	c := p.src[p.pos]
	p.pos++
	switch c {
	case 'A':
		return &ast{kind: kBegin}
	case 'Z':
		return &ast{kind: kEndStr}
	case 'b':
		return &ast{kind: kWordB}
	case 'B':
		return &ast{kind: kNotWordB}
	case 'd', 'D', 'w', 'W', 's', 'S':
		return &ast{kind: kClass, cls: &classAST{cats: []byte{byte(c)}}}
	}
	if c >= '1' && c <= '9' {
		// three octal digits are an octal escape; anything else is a backreference
		if c <= '7' && p.pos+1 < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '7' &&
			p.src[p.pos+1] >= '0' && p.src[p.pos+1] <= '7' {
			v := (c-'0')*64 + (p.src[p.pos]-'0')*8 + p.src[p.pos+1] - '0'
			p.pos += 2
			if v > 0o377 {
				p.fail("octal escape value outside of range 0-0o377")
			}
			return &ast{kind: kLit, r: v}
		}
		p.fail("backreferences are not supported")
	}
	return &ast{kind: kLit, r: p.charEscape(c)}
}

// charEscape reads the rest of an escape that stands for one character (inside or outside a class).
func (p *parser) charEscape(c rune) rune {
	switch c {
	case 'a':
		return 0x07
	case 'f':
		return 0x0C
	case 'n':
		return 0x0A
	case 'r':
		return 0x0D
	case 't':
		return 0x09
	case 'v':
		return 0x0B
	case '\\':
		return '\\'
	case 'x':
		return p.hex(2)
	case 'u':
		return p.hex(4)
	case 'U':
		r := p.hex(8)
		if r > 0x10FFFF {
			p.fail("bad escape \\U")
		}
		return r
	case '0':
		v := rune(0)
		for k := 0; k < 2 && p.more() && p.peek() >= '0' && p.peek() <= '7'; k++ {
			v = v*8 + p.src[p.pos] - '0'
			p.pos++
		}
		return v
	}
	if c < 0x80 && (c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		p.fail(fmt.Sprintf("bad escape \\%c", c))
	}
	return c
}

func (p *parser) hex(n int) rune {
	if p.pos+n > len(p.src) {
		p.fail("incomplete escape")
	}
	v, err := strconv.ParseUint(string(p.src[p.pos:p.pos+n]), 16, 32)
	if err != nil || strings.ContainsAny(string(p.src[p.pos:p.pos+n]), "+-_") {
		p.fail("incomplete escape")
	}
	p.pos += n
	return rune(v)
}

// class reads a character class after "[".
func (p *parser) class() *classAST {
	cls := &classAST{}
	if p.accept('^') {
		cls.neg = true
	}
	empty := true
	for {
		if !p.more() {
			p.fail("unterminated character set")
		}
		c := p.src[p.pos]
		p.pos++
		if c == ']' && !empty {
			break
		}
		empty = false
		lo, isCat := p.classItem(c, cls)
		if isCat {
			if p.peek() == '-' && p.pos+1 < len(p.src) && p.src[p.pos+1] != ']' {
				p.fail("bad character range")
			}
			continue
		}
		if p.peek() == '-' {
			p.pos++
			if !p.more() {
				p.fail("unterminated character set")
			}
			d := p.src[p.pos]
			p.pos++
			if d == ']' {
				cls.ranges = append(cls.ranges, [2]rune{lo, lo}, [2]rune{'-', '-'})
				break
			}
			hi, isCat := p.classItem(d, nil)
			if isCat {
				p.fail("bad character range")
			}
			if hi < lo {
				p.fail("bad character range")
			}
			cls.ranges = append(cls.ranges, [2]rune{lo, hi})
			continue
		}
		cls.ranges = append(cls.ranges, [2]rune{lo, lo})
	}
	return cls
}

// classItem reads one class member that starts with c; a category (\d, \w, ...) is added to cls and reported as such.
func (p *parser) classItem(c rune, cls *classAST) (rune, bool) {
	if c != '\\' {
		return c, false
	}
	if !p.more() {
		p.fail("bad escape (end of pattern)")
	}
	e := p.src[p.pos]
	p.pos++
	switch e {
	case 'd', 'D', 'w', 'W', 's', 'S':
		if cls == nil {
			return 0, true
		}
		cls.cats = append(cls.cats, byte(e))
		return 0, true
	case 'b':
		return 0x08, false
	}
	if e >= '1' && e <= '7' {
		// octal escape of up to three digits
		v := e - '0'
		for k := 0; k < 2 && p.more() && p.peek() >= '0' && p.peek() <= '7'; k++ {
			v = v*8 + p.src[p.pos] - '0'
			p.pos++
		}
		if v > 0o377 {
			p.fail("octal escape value outside of range 0-0o377")
		}
		return v, false
	}
	if e == '8' || e == '9' {
		p.fail(fmt.Sprintf("bad escape \\%c", e))
	}
	return p.charEscape(e), false
}

// width is the minimum and maximum number of characters the sub-pattern matches (max == inf: unbounded).
func width(a *ast) (int, int) {
	switch a.kind {
	case kLit, kClass, kAny:
		return 1, 1
	case kBegin, kEnd, kEndStr, kWordB, kNotWordB, kLook:
		return 0, 0
	case kGroup:
		return width(a.subs[0])
	case kConcat:
		lo, hi := 0, 0
		for _, s := range a.subs {
			l, h := width(s)
			lo += l
			if hi != inf {
				if h == inf {
					hi = inf
				} else {
					hi += h
				}
			}
		}
		return lo, hi
	case kAlt:
		lo, hi := -1, 0
		for _, s := range a.subs {
			l, h := width(s)
			if lo < 0 || l < lo {
				lo = l
			}
			if hi != inf && (h == inf || h > hi) {
				hi = h
			}
		}
		return lo, hi
	case kRepeat:
		l, h := width(a.subs[0])
		lo := l * a.min
		switch {
		case h == 0:
			return lo, 0
		case h == inf || a.max == inf:
			return lo, inf
		}
		return lo, h * a.max
	}
	return 0, 0
}
