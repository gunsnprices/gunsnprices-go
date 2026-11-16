package gunsnprices

import (
	"strings"
	"unicode/utf8"

	"github.com/gunsnprices/gunsnprices-go/internal/pyre"
)

// The rules are Python regular expressions, run by internal/pyre with Python's semantics.

func mustRx(pattern string, ignoreCase bool) *pyre.Regexp {
	return pyre.MustCompile(pattern, ignoreCase)
}

func mustTemplate(repl string, re *pyre.Regexp) *pyre.Template {
	t, err := pyre.ParseTemplate(repl, re.NumGroups())
	if err != nil {
		panic(err)
	}
	return t
}

var (
	emptyTemplate = &pyre.Template{}
	hashTemplate  = mustTemplate("#", mustRx("x", false))
)

// sub is Python's re.sub(pattern, template, s).
func sub(re *pyre.Regexp, s string, t *pyre.Template) string { return re.SubString(s, t) }

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// clean makes s valid UTF-8 (Python strings cannot hold invalid bytes): every invalid sequence becomes U+FFFD.
func clean(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, string(utf8.RuneError))
}
