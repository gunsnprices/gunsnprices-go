package pyre

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Unescape is Python's html.unescape: HTML5 named references with and without ";", the longest known prefix of an
// unknown name ("&ampx" -> "&x"), numeric references remapped or dropped the way browsers do.
//
// Python finds the references with the pattern
//
//	&(#[0-9]+;?|#[xX][0-9a-fA-F]+;?|[^\t\n\f <&#;]{1,32};?)
//
// and this function scans for the same thing by hand.
func Unescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for {
		k := strings.IndexByte(s[i:], '&')
		if k < 0 {
			b.WriteString(s[i:])
			break
		}
		amp := i + k
		b.WriteString(s[i:amp])
		ref, n := charref(s[amp+1:])
		if n == 0 {
			b.WriteByte('&')
			i = amp + 1
			continue
		}
		b.WriteString(replaceCharref(ref))
		i = amp + 1 + n
	}
	return b.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// charref matches the reference after "&" at the start of s; it returns the reference and its length in bytes
// (0: no reference).
func charref(s string) (string, int) {
	if s == "" {
		return "", 0
	}
	semi := func(n int) int {
		if n < len(s) && s[n] == ';' {
			return n + 1
		}
		return n
	}
	if s[0] == '#' {
		n := 1
		if n < len(s) && (s[n] == 'x' || s[n] == 'X') {
			m := n + 1
			for m < len(s) && isHex(s[m]) {
				m++
			}
			if m > n+1 {
				end := semi(m)
				return s[:end], end
			}
		}
		// "#" followed by decimal digits (also tried when "#x" has no hex digits: then it fails, "x" is no digit)
		m := 1
		for m < len(s) && s[m] >= '0' && s[m] <= '9' {
			m++
		}
		if m > 1 {
			end := semi(m)
			return s[:end], end
		}
		return "", 0
	}
	// [^\t\n\f <&#;]{1,32};?
	n, count := 0, 0
	for n < len(s) && count < 32 {
		r, size := utf8.DecodeRuneInString(s[n:])
		if r == '\t' || r == '\n' || r == '\f' || r == ' ' || r == '<' || r == '&' || r == '#' || r == ';' {
			break
		}
		n += size
		count++
	}
	if count == 0 {
		return "", 0
	}
	end := semi(n)
	return s[:end], end
}

func replaceCharref(ref string) string {
	if ref[0] == '#' {
		var num int64
		digits := strings.TrimRight(ref[1:], ";")
		base := int64(10)
		if digits[0] == 'x' || digits[0] == 'X' {
			digits = digits[1:]
			base = 16
		}
		// numbers past U+10FFFF all give U+FFFD: stop counting there
		for i := 0; i < len(digits); i++ {
			c := digits[i]
			var d int64
			switch {
			case c >= '0' && c <= '9':
				d = int64(c - '0')
			case c >= 'a' && c <= 'f':
				d = int64(c-'a') + 10
			default:
				d = int64(c-'A') + 10
			}
			num = num*base + d
			if num > 0x10FFFF {
				num = 0x110000
			}
		}
		k := sort.Search(len(invalidCharrefKeys), func(i int) bool { return int64(invalidCharrefKeys[i]) >= num })
		if k < len(invalidCharrefKeys) && int64(invalidCharrefKeys[k]) == num {
			return invalidCharrefVals[k]
		}
		if (num >= 0xD800 && num <= 0xDFFF) || num > 0x10FFFF {
			return "�"
		}
		k = sort.Search(len(invalidCodepoints), func(i int) bool { return int64(invalidCodepoints[i]) >= num })
		if k < len(invalidCodepoints) && int64(invalidCodepoints[k]) == num {
			return ""
		}
		return string(rune(num))
	}
	ents := html5()
	if v, ok := ents[ref]; ok {
		return v
	}
	// the longest known name that the reference starts with (at least 2 characters)
	rs := []rune(ref)
	for x := len(rs) - 1; x > 1; x-- {
		if v, ok := ents[string(rs[:x])]; ok {
			return v + string(rs[x:])
		}
	}
	return "&" + ref
}
