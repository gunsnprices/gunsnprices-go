package pyre

import "sync"

type node interface {
	match(m *machine, i int) bool
}

type loopState struct {
	count int
	last  int // position where the current iteration started (empty-iteration guard)
}

// machine is the state of one match attempt.
type machine struct {
	text        []rune
	caps        []int // start, end of each group (-1: not matched); group 0 is filled in by the caller
	open        []int // start of each open group
	loops       []loopState
	start       int  // where the search started
	mustAdvance bool // reject an empty match at start (Python's finditer/sub after an empty match)
	full        bool // the match must end at the end of the text (fullmatch)
	end         int  // end of the match found
}

// ---------- characters ----------

type charNode struct {
	cm   *charMatcher
	next node
}

func (n *charNode) match(m *machine, i int) bool {
	if i < len(m.text) && n.cm.matches(m.text[i]) {
		return n.next.match(m, i+1)
	}
	return false
}

type seqNode struct {
	cms  []*charMatcher
	next node
}

func (n *seqNode) match(m *machine, i int) bool {
	if i+len(n.cms) > len(m.text) {
		return false
	}
	t := m.text[i:]
	for k, cm := range n.cms {
		if !cm.matches(t[k]) {
			return false
		}
	}
	return n.next.match(m, i+len(n.cms))
}

// ---------- assertions ----------

type beginNode struct{ next node }

func (n *beginNode) match(m *machine, i int) bool { return i == 0 && n.next.match(m, i) }

type endNode struct{ next node }

func (n *endNode) match(m *machine, i int) bool {
	l := len(m.text)
	return (i == l || (i == l-1 && m.text[i] == '\n')) && n.next.match(m, i)
}

type endStrNode struct{ next node }

func (n *endStrNode) match(m *machine, i int) bool { return i == len(m.text) && n.next.match(m, i) }

type wordBNode struct {
	neg  bool
	next node
}

// Python 3.13: neither \b nor \B matches in an empty string.
func (n *wordBNode) match(m *machine, i int) bool {
	l := len(m.text)
	if l == 0 {
		return false
	}
	before := i > 0 && IsWord(m.text[i-1])
	after := i < l && IsWord(m.text[i])
	if (before != after) == n.neg {
		return false
	}
	return n.next.match(m, i)
}

// ---------- groups ----------

type groupOpen struct {
	idx  int
	next node
}

func (n *groupOpen) match(m *machine, i int) bool {
	old := m.open[n.idx]
	m.open[n.idx] = i
	if n.next.match(m, i) {
		return true
	}
	m.open[n.idx] = old
	return false
}

type groupClose struct {
	idx  int
	next node
}

func (n *groupClose) match(m *machine, i int) bool {
	k := 2 * n.idx
	s0, e0 := m.caps[k], m.caps[k+1]
	m.caps[k], m.caps[k+1] = m.open[n.idx], i
	if n.next.match(m, i) {
		return true
	}
	m.caps[k], m.caps[k+1] = s0, e0
	return false
}

// ---------- alternation ----------

type branchNode struct {
	alts []node
	// with many branches: the branches that can start with each ASCII character, with a non-ASCII character, and
	// at the end of the text (in order)
	byASCII  *[128][]int32
	nonASCII []int32
	atEnd    []int32
}

func (n *branchNode) match(m *machine, i int) bool {
	if n.byASCII == nil {
		for _, a := range n.alts {
			if a.match(m, i) {
				return true
			}
		}
		return false
	}
	var list []int32
	switch {
	case i >= len(m.text):
		list = n.atEnd
	case m.text[i] < 0x80:
		list = n.byASCII[m.text[i]]
	default:
		list = n.nonASCII
	}
	for _, k := range list {
		if n.alts[k].match(m, i) {
			return true
		}
	}
	return false
}

// ---------- repetition ----------

// curlyNode repeats a single character.
type curlyNode struct {
	cm       *charMatcher
	min, max int
	greedy   bool
	next     node
}

func (n *curlyNode) match(m *machine, i int) bool {
	t := m.text
	if n.greedy {
		lim := len(t)
		if n.max != inf && i+n.max < lim {
			lim = i + n.max
		}
		j := i
		for j < lim && n.cm.matches(t[j]) {
			j++
		}
		for ; j >= i+n.min; j-- {
			if n.next.match(m, j) {
				return true
			}
		}
		return false
	}
	j := i
	for ; j < i+n.min; j++ {
		if j >= len(t) || !n.cm.matches(t[j]) {
			return false
		}
	}
	for {
		if n.next.match(m, j) {
			return true
		}
		if (n.max != inf && j-i >= n.max) || j >= len(t) || !n.cm.matches(t[j]) {
			return false
		}
		j++
	}
}

// loopNode repeats a sub-pattern; its body ends with a loopTail that comes back for the next iteration. Like
// Python's REPEAT/MAX_UNTIL/MIN_UNTIL, once the minimum is reached a new iteration is only tried when the previous
// one consumed something.
type loopNode struct {
	body     node
	min, max int
	greedy   bool
	slot     int
	next     node
}

func (n *loopNode) match(m *machine, i int) bool {
	saved := m.loops[n.slot]
	m.loops[n.slot] = loopState{count: 0, last: -1}
	ok := n.iterate(m, i)
	m.loops[n.slot] = saved
	return ok
}

func (n *loopNode) again(m *machine, i int, c int) bool {
	st := &m.loops[n.slot]
	if (n.max != inf && c >= n.max) || i == st.last {
		return false
	}
	last := st.last
	st.count, st.last = c+1, i
	if n.body.match(m, i) {
		return true
	}
	st = &m.loops[n.slot]
	st.count, st.last = c, last
	return false
}

func (n *loopNode) iterate(m *machine, i int) bool {
	c := m.loops[n.slot].count
	if c < n.min {
		m.loops[n.slot].count = c + 1
		if n.body.match(m, i) {
			return true
		}
		m.loops[n.slot].count = c
		return false
	}
	if n.greedy {
		if n.again(m, i, c) {
			return true
		}
		return n.next.match(m, i)
	}
	if n.next.match(m, i) {
		return true
	}
	return n.again(m, i, c)
}

type loopTail struct{ loop *loopNode }

func (n *loopTail) match(m *machine, i int) bool { return n.loop.iterate(m, i) }

// ---------- look-around ----------

type lookNode struct {
	body   node
	neg    bool
	behind bool
	width  int
	groups bool // the sub-pattern has groups: keep or drop their values like Python
	next   node
}

func (n *lookNode) match(m *machine, i int) bool {
	start := i
	if n.behind {
		start = i - n.width
		if start < 0 {
			return n.neg && n.next.match(m, i)
		}
	}
	var snap []int
	if n.groups {
		snap = append(snap, m.caps...)
	}
	ok := n.body.match(m, start)
	if n.neg {
		if n.groups {
			copy(m.caps, snap)
		}
		return !ok && n.next.match(m, i)
	}
	if !ok {
		return false
	}
	if n.next.match(m, i) {
		return true
	}
	if n.groups {
		copy(m.caps, snap)
	}
	return false
}

// subAccept ends the sub-pattern of a look-around.
type subAccept struct{}

func (subAccept) match(*machine, int) bool { return true }

// accept ends the pattern.
type accept struct{}

func (accept) match(m *machine, i int) bool {
	if m.full && i != len(m.text) {
		return false
	}
	if m.mustAdvance && i == m.start {
		return false
	}
	m.end = i
	return true
}

// ---------- Regexp ----------

// Regexp is a compiled Python pattern. It is safe for concurrent use.
type Regexp struct {
	pattern  string
	prog     node
	ngroup   int
	nloop    int
	first    firstSet
	anchored bool // every match starts at position 0
	pool     sync.Pool
}

// Compile compiles a Python re pattern; ignoreCase is re.IGNORECASE.
func Compile(pattern string, ignoreCase bool) (*Regexp, error) {
	tree, ngroup, err := parse(pattern)
	if err != nil {
		return nil, err
	}
	if err := prepare(tree, ignoreCase); err != nil {
		return nil, &Error{Pattern: pattern, Msg: err.Error()}
	}
	c := &compiler{}
	re := &Regexp{pattern: pattern, ngroup: ngroup}
	re.prog = c.compile(tree, accept{})
	re.nloop = c.nloop
	re.first = firstOf(tree)
	if t := unwrap(tree); t.kind == kBegin || (t.kind == kConcat && len(t.subs) > 0 && t.subs[0].kind == kBegin) {
		re.anchored = true
	}
	return re, nil
}

// MustCompile is Compile that panics on an error.
func MustCompile(pattern string, ignoreCase bool) *Regexp {
	re, err := Compile(pattern, ignoreCase)
	if err != nil {
		panic(err)
	}
	return re
}

// String returns the pattern.
func (re *Regexp) String() string { return re.pattern }

// NumGroups is the number of capturing groups.
func (re *Regexp) NumGroups() int { return re.ngroup }

func (re *Regexp) machine(text []rune) *machine {
	m, _ := re.pool.Get().(*machine)
	if m == nil {
		m = &machine{
			caps:  make([]int, 2*(re.ngroup+1)),
			open:  make([]int, re.ngroup+1),
			loops: make([]loopState, re.nloop),
		}
	}
	for k := range m.caps {
		m.caps[k] = -1
	}
	m.text = text
	return m
}

func (re *Regexp) release(m *machine) {
	m.text = nil
	re.pool.Put(m)
}

// run tries to match at positions from pos (search) or only at pos (anchor); it fills m.caps[0:2].
func (re *Regexp) run(m *machine, pos int, anchor, full, mustAdvance bool) bool {
	m.start, m.full, m.mustAdvance = pos, full, mustAdvance
	last := len(m.text)
	if anchor || re.anchored {
		last = pos
	}
	for i := pos; i <= last; i++ {
		if !re.first.canStart(m.text, i) {
			continue
		}
		if re.prog.match(m, i) {
			m.caps[0], m.caps[1] = i, m.end
			return true
		}
	}
	return false
}

func clamp(pos, n int) int {
	if pos < 0 {
		return 0
	}
	if pos > n {
		return n
	}
	return pos
}

func (re *Regexp) find(text []rune, pos int, anchor, full, mustAdvance bool) *Match {
	m := re.machine(text)
	defer re.release(m)
	if !re.run(m, clamp(pos, len(text)), anchor, full, mustAdvance) {
		return nil
	}
	return &Match{text: text, caps: append([]int(nil), m.caps...)}
}

// Search is Python's pattern.search(text, pos): the first match starting at pos or later. The text before pos is
// still seen by look-behind and \b, and "^" only matches at 0.
func (re *Regexp) Search(text []rune, pos int) *Match { return re.find(text, pos, false, false, false) }

// MatchAt is Python's pattern.match(text, pos): a match starting exactly at pos.
func (re *Regexp) MatchAt(text []rune, pos int) *Match { return re.find(text, pos, true, false, false) }

// FullMatch is Python's pattern.fullmatch(text).
func (re *Regexp) FullMatch(text []rune) *Match { return re.find(text, 0, true, true, false) }

// IsMatch is Python's pattern.search(text) is not None.
func (re *Regexp) IsMatch(text []rune) bool {
	m := re.machine(text)
	defer re.release(m)
	return re.run(m, 0, false, false, false)
}

// MatchString is IsMatch for a string.
func (re *Regexp) MatchString(s string) bool { return re.IsMatch([]rune(s)) }

// each calls f for every non-overlapping match, like Python's finditer: after an empty match the next one may start
// at the same position but must not be empty there.
func (re *Regexp) each(text []rune, f func(caps []int) bool) {
	m := re.machine(text)
	defer re.release(m)
	pos, mustAdvance := 0, false
	for pos <= len(text) {
		if !re.run(m, pos, false, false, mustAdvance) {
			return
		}
		if !f(m.caps) {
			return
		}
		mustAdvance = m.caps[1] == m.caps[0]
		pos = m.caps[1]
		for k := range m.caps {
			m.caps[k] = -1
		}
	}
}

// FindAll is Python's pattern.finditer(text).
func (re *Regexp) FindAll(text []rune) []*Match {
	var out []*Match
	re.each(text, func(caps []int) bool {
		out = append(out, &Match{text: text, caps: append([]int(nil), caps...)})
		return true
	})
	return out
}

// FindAllStrings is Python's pattern.findall(text) for a pattern without groups.
func (re *Regexp) FindAllStrings(text []rune) []string {
	var out []string
	re.each(text, func(caps []int) bool {
		out = append(out, string(text[caps[0]:caps[1]]))
		return true
	})
	return out
}

// Sub is Python's pattern.sub(template, text); it returns text itself when nothing matched.
func (re *Regexp) Sub(text []rune, t *Template) []rune {
	out, _ := re.sub(text, t)
	return out
}

// SubString is Sub for a string.
func (re *Regexp) SubString(s string, t *Template) string {
	out, changed := re.sub([]rune(s), t)
	if !changed {
		return s
	}
	return string(out)
}

func (re *Regexp) sub(text []rune, t *Template) ([]rune, bool) {
	var out []rune
	last := 0
	found := false
	re.each(text, func(caps []int) bool {
		found = true
		out = append(out, text[last:caps[0]]...)
		for _, p := range t.parts {
			if p.group < 0 {
				out = append(out, p.lit...)
			} else if s, e := caps[2*p.group], caps[2*p.group+1]; s >= 0 {
				out = append(out, text[s:e]...)
			}
		}
		last = caps[1]
		return true
	})
	if !found {
		return text, false
	}
	return append(out, text[last:]...), true
}

// Match is a successful match. Positions are character (code point) offsets, like Python's.
type Match struct {
	text []rune
	caps []int
}

// Start is the start of the match.
func (m *Match) Start() int { return m.caps[0] }

// End is the end of the match.
func (m *Match) End() int { return m.caps[1] }

// Span is the start and end of group i (-1, -1 if it did not take part in the match).
func (m *Match) Span(i int) (int, int) { return m.caps[2*i], m.caps[2*i+1] }

// Group is the text of group i (0: the whole match); "" if it did not take part in the match.
func (m *Match) Group(i int) string {
	s, e := m.caps[2*i], m.caps[2*i+1]
	if s < 0 {
		return ""
	}
	return string(m.text[s:e])
}

// Has reports whether group i took part in the match (Python: m.group(i) is not None).
func (m *Match) Has(i int) bool { return m.caps[2*i] >= 0 }
