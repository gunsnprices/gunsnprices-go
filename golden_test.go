package gunsnprices_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gunsnprices/gunsnprices-go"
)

// The golden test: ParseTitle on 1,150 real store titles must give the answers the Python package (the reference
// implementation) recorded in testdata/golden.jsonl. The same file is used by the Python, JavaScript, PHP, Rust,
// Elixir, Dart, .NET, Java and Ruby versions.

type goldenCase struct {
	Args struct {
		Title string   `json:"title"`
		Brand *string  `json:"brand"`
		MPN   *string  `json:"mpn"`
		SKU   *string  `json:"sku"`
		Price *float64 `json:"price"`
		Kind  *string  `json:"kind"`
	} `json:"args"`
	Expected map[string]any `json:"expected"`
}

func loadGolden(t *testing.T) []goldenCase {
	t.Helper()
	data, err := os.ReadFile("testdata/golden.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var c goldenCase
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	return cases
}

// compact is the golden file's summary of a ProductInfo; numbers keep their Go types (int or float64).
func compact(info gunsnprices.ProductInfo) map[string]any {
	var model any
	if m := info.Model; m != nil {
		model = []any{m.Family, m.Model, m.Generation, strs(m.Configs), strs(m.Features), m.Source}
	}
	ids := []any{}
	for _, c := range info.Calibers {
		ids = append(ids, c.ID)
	}
	out := map[string]any{
		"kind": string(info.Kind), "brand": info.Brand, "model": model, "calibers": ids,
		"capacity": nil, "barrel_length": nil, "rounds": nil, "cost_per_round": nil,
	}
	if info.Capacity != nil {
		out["capacity"] = *info.Capacity
	}
	if info.BarrelLength != nil {
		out["barrel_length"] = *info.BarrelLength
	}
	if info.Rounds != nil {
		out["rounds"] = *info.Rounds
	}
	if info.CostPerRound != nil {
		out["cost_per_round"] = *info.CostPerRound
	}
	return out
}

func strs(l []string) []any {
	out := []any{}
	for _, s := range l {
		out = append(out, s)
	}
	return out
}

// same compares strictly: a JSON integer must be a Go int and a JSON float (26.0) a Go float64, as in Python where
// the recorded values are int and float.
func same(want, got any) bool {
	switch w := want.(type) {
	case nil:
		return got == nil
	case json.Number:
		if strings.ContainsAny(string(w), ".eE") {
			g, ok := got.(float64)
			f, err := w.Float64()
			return ok && err == nil && f == g
		}
		g, ok := got.(int)
		i, err := w.Int64()
		return ok && err == nil && int64(g) == i
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !same(w[i], g[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for k, v := range w {
			gv, ok := g[k]
			if !ok || !same(v, gv) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprintf("%T %v", want, want) == fmt.Sprintf("%T %v", got, got)
	}
}

func optionsOf(c goldenCase) *gunsnprices.Options {
	o := &gunsnprices.Options{}
	if c.Args.Brand != nil {
		o.Brand = *c.Args.Brand
	}
	if c.Args.MPN != nil {
		o.MPN = *c.Args.MPN
	}
	if c.Args.SKU != nil {
		o.SKU = *c.Args.SKU
	}
	if c.Args.Price != nil {
		o.Price = *c.Args.Price
	}
	if c.Args.Kind != nil {
		o.Kind = gunsnprices.Kind(*c.Args.Kind)
	}
	return o
}

func TestGolden(t *testing.T) {
	cases := loadGolden(t)
	if len(cases) != 1150 {
		t.Fatalf("golden.jsonl has %d cases, want 1150", len(cases))
	}
	failed := 0
	for i, c := range cases {
		got := compact(gunsnprices.ParseTitle(c.Args.Title, optionsOf(c)))
		if !same(c.Expected, got) {
			failed++
			if failed <= 20 {
				var diff []string
				for k, w := range c.Expected {
					if !same(w, got[k]) {
						diff = append(diff, fmt.Sprintf("%s: got %#v, want %#v", k, got[k], w))
					}
				}
				t.Errorf("case %d %q:\n  %s", i, c.Args.Title, strings.Join(diff, "\n  "))
			}
		}
	}
	t.Logf("golden: %d/%d titles parse exactly like the Python package", len(cases)-failed, len(cases))
}
