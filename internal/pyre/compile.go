package pyre

// The compiled form is a graph of nodes in the style of a backtracking matcher: every node matches its own piece at
// position i and then calls next; a node returns false to backtrack. Alternation tries its branches in order and
// repetition tries the longest (greedy) or shortest (lazy) count first, so the first match found is the one Python's
// re module finds (leftmost-first, the same backtracking order).

// firstSet describes the characters a sub-pattern can start with.
type firstSet struct {
	ascii    [2]uint64
	nonASCII bool
	nullable bool // may match without consuming a character (then any position is possible)
}

func (f *firstSet) union(g firstSet) {
	f.ascii[0] |= g.ascii[0]
	f.ascii[1] |= g.ascii[1]
	f.nonASCII = f.nonASCII || g.nonASCII
}

// canStart reports whether a match could start at text[i].
func (f *firstSet) canStart(text []rune, i int) bool {
	if f.nullable {
		return true
	}
	if i >= len(text) {
		return false
	}
	r := text[i]
	if r < 0x80 {
		return f.ascii[r>>6]&(1<<(uint(r)&63)) != 0
	}
	return f.nonASCII
}

func firstOf(a *ast) firstSet {
	switch a.kind {
	case kLit, kClass, kAny:
		return firstSet{ascii: a.cm.ascii, nonASCII: a.cm.nonASCII}
	case kBegin, kEnd, kEndStr, kWordB, kNotWordB, kLook:
		return firstSet{nullable: true}
	case kGroup:
		return firstOf(a.subs[0])
	case kConcat:
		f := firstSet{nullable: true}
		for _, s := range a.subs {
			g := firstOf(s)
			f.union(g)
			if !g.nullable {
				f.nullable = false
				break
			}
		}
		return f
	case kAlt:
		var f firstSet
		for _, s := range a.subs {
			g := firstOf(s)
			f.union(g)
			f.nullable = f.nullable || g.nullable
		}
		return f
	case kRepeat:
		f := firstOf(a.subs[0])
		if a.min == 0 {
			f.nullable = true
		}
		return f
	}
	return firstSet{nullable: true}
}

// prepare builds the character matchers of the tree.
func prepare(a *ast, ignoreCase bool) error {
	switch a.kind {
	case kLit:
		a.cm = literalMatcher(a.r, ignoreCase)
	case kAny:
		a.cm = anyMatcher
	case kClass:
		cm, err := classMatcher(a.cls, ignoreCase)
		if err != nil {
			return err
		}
		a.cm = cm
	}
	for _, s := range a.subs {
		if err := prepare(s, ignoreCase); err != nil {
			return err
		}
	}
	return nil
}

func isChar(a *ast) bool { return a.kind == kLit || a.kind == kClass || a.kind == kAny }

// unwrap returns the only element of a one-element sequence.
func unwrap(a *ast) *ast {
	for a.kind == kConcat && len(a.subs) == 1 {
		a = a.subs[0]
	}
	return a
}

func hasGroup(a *ast) bool {
	if a.kind == kGroup {
		return true
	}
	for _, s := range a.subs {
		if hasGroup(s) {
			return true
		}
	}
	return false
}

type compiler struct {
	nloop int
}

func (c *compiler) compile(a *ast, next node) node {
	switch a.kind {
	case kLit, kClass, kAny:
		return &charNode{cm: a.cm, next: next}
	case kBegin:
		return &beginNode{next: next}
	case kEnd:
		return &endNode{next: next}
	case kEndStr:
		return &endStrNode{next: next}
	case kWordB:
		return &wordBNode{next: next}
	case kNotWordB:
		return &wordBNode{neg: true, next: next}
	case kGroup:
		return &groupOpen{idx: a.idx, next: c.compile(a.subs[0], &groupClose{idx: a.idx, next: next})}
	case kLook:
		return &lookNode{
			body: c.compile(a.subs[0], subAccept{}), neg: a.neg, behind: a.behind, width: a.width,
			groups: hasGroup(a.subs[0]), next: next,
		}
	case kConcat:
		n := next
		for i := len(a.subs) - 1; i >= 0; {
			// a run of single characters becomes one node
			j := i
			for j >= 0 && isChar(a.subs[j]) {
				j--
			}
			if i-j >= 2 {
				cms := make([]*charMatcher, 0, i-j)
				for k := j + 1; k <= i; k++ {
					cms = append(cms, a.subs[k].cm)
				}
				n = &seqNode{cms: cms, next: n}
				i = j
				continue
			}
			n = c.compile(a.subs[i], n)
			i--
		}
		return n
	case kAlt:
		b := &branchNode{alts: make([]node, len(a.subs))}
		firsts := make([]firstSet, len(a.subs))
		for i, s := range a.subs {
			b.alts[i] = c.compile(s, next)
			firsts[i] = firstOf(s)
		}
		if len(a.subs) >= 6 {
			b.buildDispatch(firsts)
		}
		return b
	case kRepeat:
		sub := unwrap(a.subs[0])
		if a.max == 0 {
			return next
		}
		if a.min == 1 && a.max == 1 {
			return c.compile(sub, next)
		}
		if isChar(sub) {
			return &curlyNode{cm: sub.cm, min: a.min, max: a.max, greedy: a.greedy, next: next}
		}
		l := &loopNode{min: a.min, max: a.max, greedy: a.greedy, slot: c.nloop, next: next}
		c.nloop++
		l.body = c.compile(sub, &loopTail{loop: l})
		return l
	}
	panic("pyre: unknown node")
}

func (b *branchNode) buildDispatch(firsts []firstSet) {
	b.byASCII = new([128][]int32)
	for r := 0; r < 128; r++ {
		for i, f := range firsts {
			if f.nullable || f.ascii[r>>6]&(1<<(uint(r)&63)) != 0 {
				b.byASCII[r] = append(b.byASCII[r], int32(i))
			}
		}
	}
	for i, f := range firsts {
		if f.nullable || f.nonASCII {
			b.nonASCII = append(b.nonASCII, int32(i))
		}
		if f.nullable {
			b.atEnd = append(b.atEnd, int32(i))
		}
	}
}
