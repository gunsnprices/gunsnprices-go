package pyre

import (
	"sort"
	"strings"
	"sync"
)

// UnicodeVersion is the Unicode version of the character tables (Python 3.13's), which may differ from the version
// of Go's unicode package.
const UnicodeVersion = unicodeVersion

// inRanges reports whether r is in a sorted list of inclusive ranges (first, last, first, last, ...).
func inRanges(ranges []int32, r rune) bool {
	// the first range whose last element is >= r
	n := len(ranges) / 2
	i := sort.Search(n, func(k int) bool { return ranges[2*k+1] >= r })
	return i < n && ranges[2*i] <= r
}

// IsSpace is Python's str.isspace() for one character (also what \s matches).
func IsSpace(r rune) bool {
	if r < 0x80 {
		return r == ' ' || (r >= 0x09 && r <= 0x0D) || (r >= 0x1C && r <= 0x1F)
	}
	return inRanges(spaceRanges, r)
}

// IsWord is Python's \w for one character: str.isalnum() or "_".
func IsWord(r rune) bool {
	if r < 0x80 {
		return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return inRanges(wordRanges, r)
}

// DigitValue is the value of a Unicode decimal digit (Python's \d, int()), or -1.
func DigitValue(r rune) int {
	if r >= '0' && r <= '9' {
		return int(r - '0')
	}
	if r < 0x80 {
		return -1
	}
	i := sort.Search(len(digitZeros), func(k int) bool { return digitZeros[k] > r }) - 1
	if i >= 0 && r-digitZeros[i] < 10 {
		return int(r - digitZeros[i])
	}
	return -1
}

// IsDecimal is Python's \d for one character.
func IsDecimal(r rune) bool { return DigitValue(r) >= 0 }

// IsDigit is Python's str.isdigit() for one character (decimal digits, superscripts, circled digits, ...).
func IsDigit(r rune) bool {
	if r < 0x80 {
		return r >= '0' && r <= '9'
	}
	return inRanges(isdigitRanges, r)
}

func lookupString(keys []int32, vals []string, r rune) (string, bool) {
	i := sort.Search(len(keys), func(k int) bool { return keys[k] >= r })
	if i < len(keys) && keys[i] == r {
		return vals[i], true
	}
	return "", false
}

// sreLower is _sre.unicode_tolower: the simple lowercase mapping that re.IGNORECASE compares.
func sreLower(r rune) rune {
	if r < 0x80 {
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		return r
	}
	i := sort.Search(len(sreLowerKeys), func(k int) bool { return sreLowerKeys[k] >= r })
	if i < len(sreLowerKeys) && sreLowerKeys[i] == r {
		return sreLowerVals[i]
	}
	return r
}

// sreIsCased is _sre.unicode_iscased.
func sreIsCased(r rune) bool {
	if r < 0x80 {
		return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
	}
	return inRanges(sreCasedRanges, r)
}

// caseFixes are the characters re.IGNORECASE also matches for a lowercase character (re._casefix).
func caseFixes(lo rune) []rune {
	i := sort.Search(len(caseFixKeys), func(k int) bool { return caseFixKeys[k] >= lo })
	if i < len(caseFixKeys) && caseFixKeys[i] == lo {
		return []rune(caseFixVals[i])
	}
	return nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Lower is Python's str.lower(): the full Unicode lowercase mapping, with the final sigma ("ΑΣ" -> "ας").
func Lower(s string) string {
	if isASCII(s) {
		return strings.ToLower(s)
	}
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range rs {
		switch {
		case r < 0x80:
			if r >= 'A' && r <= 'Z' {
				r += 32
			}
			b.WriteRune(r)
		case r == 0x3A3:
			if finalSigma(rs, i) {
				b.WriteRune(0x3C2)
			} else {
				b.WriteRune(0x3C3)
			}
		default:
			if v, ok := lookupString(lowerKeys, lowerVals, r); ok {
				b.WriteString(v)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// finalSigma: U+03A3 at rs[i] is a final sigma when, skipping case-ignorable characters, a cased character comes
// before it and none after it (CPython's handle_capital_sigma).
func finalSigma(rs []rune, i int) bool {
	j := i - 1
	for j >= 0 && inRanges(caseIgnorableRanges, rs[j]) {
		j--
	}
	if j < 0 || !inRanges(casedRanges, rs[j]) {
		return false
	}
	j = i + 1
	for j < len(rs) && inRanges(caseIgnorableRanges, rs[j]) {
		j++
	}
	return j == len(rs) || !inRanges(casedRanges, rs[j])
}

// Upper is Python's str.upper(): the full Unicode uppercase mapping ("ß" -> "SS").
func Upper(s string) string {
	if isASCII(s) {
		return strings.ToUpper(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r < 0x80:
			if r >= 'a' && r <= 'z' {
				r -= 32
			}
			b.WriteRune(r)
		default:
			if v, ok := lookupString(upperKeys, upperVals, r); ok {
				b.WriteString(v)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// Strip is Python's str.strip().
func Strip(s string) string {
	return strings.TrimFunc(s, IsSpace)
}

// CollapseSpace is Python's re.sub(r"\s+", " ", s).
func CollapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	changed := false
	for _, r := range s {
		if IsSpace(r) {
			if inSpace {
				changed = true
				continue
			}
			inSpace = true
			if r != ' ' {
				changed = true
			}
			b.WriteByte(' ')
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	if !changed {
		return s
	}
	return b.String()
}

// Fields is Python's [t for t in re.split(r"\s+", s.strip()) if t].
func Fields(s string) []string {
	return strings.FieldsFunc(s, IsSpace)
}

// Escape is Python's re.escape (3.7+): the same characters get a backslash.
func Escape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		if r < 0x80 && strings.ContainsRune("()[]{}?*+-|^$\\.&~# \t\n\r\v\f", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// maxInt is returned by Int for numbers that do not fit; the callers only compare against small limits.
const maxInt = int(^uint(0) >> 1)

// Int is Python's int(s) for a string of decimal digits of any script (what \d matches). Numbers too large for an
// int give the largest int. ok is false if s is empty or has a character that is not a decimal digit.
func Int(s string) (n int, ok bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		d := DigitValue(r)
		if d < 0 {
			return 0, false
		}
		if n > (maxInt-d)/10 {
			n = maxInt
			continue
		}
		n = n*10 + d
	}
	return n, true
}

// ASCIIDigits returns s with every Unicode decimal digit replaced by its ASCII digit (Python's float() and int()
// read digits of any script).
func ASCIIDigits(s string) string {
	if isASCII(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if d := DigitValue(r); d >= 0 {
			b.WriteByte(byte('0' + d))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	html5Once sync.Once
	html5Map  map[string]string
)

func html5() map[string]string {
	html5Once.Do(func() {
		parts := strings.Split(html5Entities, "\x00")
		html5Map = make(map[string]string, len(parts)/2)
		for i := 0; i+1 < len(parts); i += 2 {
			html5Map[parts[i]] = parts[i+1]
		}
	})
	return html5Map
}
