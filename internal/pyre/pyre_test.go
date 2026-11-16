package pyre

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

type pyCase struct {
	P      string     `json:"p"`
	I      bool       `json:"i"`
	T      string     `json:"t"`
	Search [][2]int   `json:"search"`
	Pos    int        `json:"pos"`
	Match  [][2]int   `json:"match"`
	Full   [][2]int   `json:"full"`
	Iter   [][][2]int `json:"iter"`
	Tmpl   string     `json:"tmpl"`
	Sub    string     `json:"sub"`
}

func spans(m *Match, n int) [][2]int {
	if m == nil {
		return nil
	}
	out := make([][2]int, n+1)
	for g := 0; g <= n; g++ {
		s, e := m.Span(g)
		out[g] = [2]int{s, e}
	}
	return out
}

// TestAgainstPython compares the engine with Python's re on testdata/cases.jsonl.gz (tools/gen_pyre_cases.py).
func TestAgainstPython(t *testing.T) {
	f, err := os.Open("testdata/cases.jsonl.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n, bad := 0, 0
	cache := map[string]*Regexp{}
	for sc.Scan() {
		var c pyCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		n++
		key := fmt.Sprint(c.I, c.P)
		re := cache[key]
		if re == nil {
			re, err = Compile(c.P, c.I)
			if err != nil {
				t.Fatalf("compile %q: %v", c.P, err)
			}
			cache[key] = re
		}
		text := []rune(c.T)
		var iter [][][2]int
		for _, m := range re.FindAll(text) {
			iter = append(iter, spans(m, re.NumGroups()))
		}
		tmpl, err := ParseTemplate(c.Tmpl, re.NumGroups())
		if err != nil {
			t.Fatal(err)
		}
		got := pyCase{
			P: c.P, I: c.I, T: c.T, Pos: c.Pos, Tmpl: c.Tmpl,
			Search: spans(re.Search(text, 0), re.NumGroups()),
			Match:  spans(re.MatchAt(text, c.Pos), re.NumGroups()),
			Full:   spans(re.FullMatch(text), re.NumGroups()),
			Iter:   iter,
			Sub:    string(re.Sub(text, tmpl)),
		}
		if len(c.Iter) == 0 {
			c.Iter = nil
		}
		if !reflect.DeepEqual(got, c) {
			bad++
			if bad <= 20 {
				t.Errorf("pattern %q (ignorecase %v) on %q:\n  python %+v\n  go     %+v", c.P, c.I, c.T, c, got)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n < 1000 {
		t.Fatalf("only %d cases", n)
	}
	t.Logf("%d cases, %d differences", n, bad)
}

func TestStringHelpers(t *testing.T) {
	sigma := string(rune(0x3A3))
	cases := []struct{ got, want string }{
		{Lower("ΑΣ"), "ας"},
		{Lower("Α" + sigma + "Α"), "ασα"},
		{Lower("İ"), "i" + string(rune(0x307))},
		{Upper("ß"), "SS"},
		{Upper("ſ"), "S"},
		{Strip(" \t\x1c a b\u3000"), "a b"},
		{CollapseSpace(" a \t\x1cb  "), " a b "},
		{Escape("a b-c&d~e#f.g"), `a\ b\-c\&d\~e\#f\.g`},
		{ASCIIDigits("١٥.5"), "15.5"},
		{Unescape("&amp;quot; &#0; &#x110000; &ampfoo &notit; &#x80; &#128; &#xD800; &AElig &#65 &#x41 a&b &; &#; &#x;"),
			"&quot; \uFFFD \uFFFD &foo ¬it; € € \uFFFD Æ A A a&b &; &#; &#x;"},
		{Unescape("&#99999999999999999999;"), "\uFFFD"},
		{Unescape("x &frac12; y &NotNestedGreaterGreater;"), "x ½ y ⪢" + string(rune(0x338))},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("%d: got %q, want %q", i, c.got, c.want)
		}
	}
	if v, ok := Int("١٥"); !ok || v != 15 {
		t.Errorf("Int: %d %v", v, ok)
	}
	if v, ok := Int("99999999999999999999999"); !ok || v != maxInt {
		t.Errorf("Int overflow: %d %v", v, ok)
	}
	if !IsWord('½') || IsWord(rune(0x301)) || !IsSpace(0x1c) || IsSpace(0x200b) {
		t.Error("character classes")
	}
}

func TestCompileErrors(t *testing.T) {
	for _, p := range []string{`(a`, `a)`, `*a`, `a**`, `[a`, `\1`, `(?<=a+)b`, `(?i)a`, `a++`, `\q`, `(?<n>a)`, `\b*`} {
		if _, err := Compile(p, false); err == nil {
			t.Errorf("%q: expected an error", p)
		}
	}
}
