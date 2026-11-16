package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestTitleArgument(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"Glock", "19", "Gen", "5", "9mm"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var v struct {
		Model struct {
			Model string `json:"model"`
		} `json:"model"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil || v.Model.Model != "Glock 19 Gen 5" {
		t.Errorf("%v %s", err, out.String())
	}
	if !strings.HasPrefix(out.String(), "{\n  \"title\": \"Glock 19 Gen 5 9mm\",\n") {
		t.Errorf("not indented: %s", out.String())
	}
}

func TestNoHTMLEscaping(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"Smith & Wesson M&P Shield Plus <b>9mm</b>"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	// like Python's json.dumps(..., ensure_ascii=False)
	for _, want := range []string{`"title": "Smith & Wesson M&P Shield Plus <b>9mm</b>"`, `"model": "S&W M&P Shield Plus"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %s in %s", want, out.String())
		}
	}
}

func TestStdin(t *testing.T) {
	var out, errOut bytes.Buffer
	in := strings.NewReader("Federal 9mm 115gr FMJ 50/Box\n\n  Winchester 308 150gr 20rds  \r\nno newline at the end")
	if code := run([]string{"-price", "18.99"}, in, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines: %s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], `"cost_per_round":0.3798`) || !strings.Contains(lines[1], `"title":"Winchester 308 150gr 20rds"`) {
		t.Errorf("%s", out.String())
	}
}

func TestFlags(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-version"}, nil, &out, &errOut); code != 0 || !strings.HasPrefix(out.String(), "gunsnprices ") {
		t.Errorf("version: %d %q", code, out.String())
	}
	if code := run([]string{"-kind", "pistol", "x"}, nil, &out, &errOut); code != 2 {
		t.Errorf("bad kind: exit %d", code)
	}
}
