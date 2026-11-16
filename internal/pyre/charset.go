package pyre

import (
	"fmt"
	"sort"
)

// charMatcher tests one character. ASCII characters are looked up in a bitmap; others go through slow.
type charMatcher struct {
	ascii    [2]uint64
	nonASCII bool // may match a character >= 0x80 (used for first-character sets)
	slow     func(r rune) bool
}

func (cm *charMatcher) matches(r rune) bool {
	if r < 0x80 {
		return cm.ascii[r>>6]&(1<<(uint(r)&63)) != 0
	}
	return cm.slow(r)
}

func newMatcher(pred func(r rune) bool, nonASCII bool) *charMatcher {
	cm := &charMatcher{slow: pred, nonASCII: nonASCII}
	for r := rune(0); r < 0x80; r++ {
		if pred(r) {
			cm.ascii[r>>6] |= 1 << (uint(r) & 63)
		}
	}
	return cm
}

// lowersToASCII are the characters >= 0x80 whose re.IGNORECASE lowercase is ASCII ("İ" -> "i", Kelvin sign -> "k").
var lowersToASCII = func() map[rune]bool {
	m := map[rune]bool{}
	for i, k := range sreLowerKeys {
		if k >= 0x80 && sreLowerVals[i] < 0x80 {
			m[sreLowerVals[i]] = true
		}
	}
	return m
}()

// runeSet is a sorted list of disjoint inclusive ranges.
type runeSet [][2]rune

func makeRuneSet(rs [][2]rune) runeSet {
	s := append([][2]rune(nil), rs...)
	sort.Slice(s, func(i, j int) bool { return s[i][0] < s[j][0] })
	var out runeSet
	for _, r := range s {
		if n := len(out); n > 0 && r[0] <= out[n-1][1]+1 {
			if r[1] > out[n-1][1] {
				out[n-1][1] = r[1]
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func (s runeSet) contains(r rune) bool {
	if len(s) <= 8 {
		for _, x := range s {
			if r < x[0] {
				return false
			}
			if r <= x[1] {
				return true
			}
		}
		return false
	}
	i := sort.Search(len(s), func(k int) bool { return s[k][1] >= r })
	return i < len(s) && s[i][0] <= r
}

func (s runeSet) hasNonASCII() bool {
	return len(s) > 0 && s[len(s)-1][1] >= 0x80
}

func (s runeSet) anyLowersFromNonASCII() bool {
	for r := range lowersToASCII {
		if s.contains(r) {
			return true
		}
	}
	return false
}

func category(c byte) func(rune) bool {
	switch c {
	case 'd':
		return IsDecimal
	case 'D':
		return func(r rune) bool { return !IsDecimal(r) }
	case 's':
		return IsSpace
	case 'S':
		return func(r rune) bool { return !IsSpace(r) }
	case 'w':
		return IsWord
	default: // 'W'
		return func(r rune) bool { return !IsWord(r) }
	}
}

// literalMatcher: with re.IGNORECASE a cased character c matches t when lower(t) is lower(c) or one of the extra
// case pairs of lower(c) (re._compiler: LITERAL_UNI_IGNORE / IN_UNI_IGNORE); otherwise t must be c.
func literalMatcher(c rune, ignoreCase bool) *charMatcher {
	if !ignoreCase || !sreIsCased(c) {
		return newMatcher(func(r rune) bool { return r == c }, c >= 0x80)
	}
	lo := sreLower(c)
	set := append([]rune{lo}, caseFixes(lo)...)
	nonASCII := lowersToASCII[lo]
	for _, x := range set {
		nonASCII = nonASCII || x >= 0x80
	}
	return newMatcher(func(r rune) bool {
		l := sreLower(r)
		for _, x := range set {
			if l == x {
				return true
			}
		}
		return false
	}, nonASCII)
}

var anyMatcher = newMatcher(func(r rune) bool { return r != '\n' }, true)

// classMatcher follows re._compiler._optimize_charset: with re.IGNORECASE, when the class has a cased member, the
// members are lowered (plus their extra case pairs) and the lowered text character is tested; otherwise the text
// character is tested as is. Categories (\d, \w, \s) give the same answer either way.
func classMatcher(cls *classAST, ignoreCase bool) (*charMatcher, error) {
	cats := make([]func(rune) bool, len(cls.cats))
	for i, c := range cls.cats {
		cats[i] = category(c)
	}
	set := makeRuneSet(cls.ranges)
	lower := false
	if ignoreCase {
		var lowered [][2]rune
		for _, rg := range cls.ranges {
			if rg[1] > 0xFFFF {
				for r := rg[0]; r <= rg[1]; r++ {
					if sreIsCased(r) {
						return nil, fmt.Errorf("case-insensitive class with a cased character outside the BMP")
					}
				}
			}
			for r := rg[0]; r <= rg[1]; r++ {
				if rg[0] == rg[1] {
					// a single character: Python tests whether its lowercase is cased
					lower = lower || sreIsCased(sreLower(r))
				} else {
					lower = lower || sreIsCased(r)
				}
				l := sreLower(r)
				lowered = append(lowered, [2]rune{l, l})
				for _, f := range caseFixes(l) {
					lowered = append(lowered, [2]rune{f, f})
				}
			}
		}
		if lower {
			set = makeRuneSet(lowered)
		}
	}
	neg := cls.neg
	pred := func(r rune) bool {
		if lower {
			r = sreLower(r)
		}
		in := set.contains(r)
		for _, c := range cats {
			if in {
				break
			}
			in = c(r)
		}
		return in != neg
	}
	nonASCII := neg || len(cats) > 0 || set.hasNonASCII() || (lower && set.anyLowersFromNonASCII())
	return newMatcher(pred, nonASCII), nil
}
