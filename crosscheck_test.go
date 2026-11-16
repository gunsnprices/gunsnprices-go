package gunsnprices_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gunsnprices/gunsnprices-go"
	"github.com/gunsnprices/gunsnprices-go/internal/pyre"
)

// TestCrosscheck compares every public function with the Python package (the reference implementation) on a corpus
// made by tools/make_corpus.py. It runs only when GUNSNPRICES_CORPUS names the corpus file:
//
//	python3.13 tools/make_corpus.py /tmp/corpus.jsonl --catalog ../library/testdata
//	GUNSNPRICES_CORPUS=/tmp/corpus.jsonl go test -run Crosscheck -v
func TestCrosscheck(t *testing.T) {
	path := os.Getenv("GUNSNPRICES_CORPUS")
	if path == "" {
		t.Skip("set GUNSNPRICES_CORPUS to a corpus made by tools/make_corpus.py")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<22), 1<<22)
	records, shown := 0, 0
	bad := map[string]int{}
	started := time.Now()
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec struct {
			In struct {
				Title string   `json:"title"`
				Brand *string  `json:"brand"`
				MPN   string   `json:"mpn"`
				SKU   string   `json:"sku"`
				Price *float64 `json:"price"`
			} `json:"in"`
			Out map[string]json.RawMessage `json:"out"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		records++
		in := rec.In
		o := &gunsnprices.Options{MPN: in.MPN, SKU: in.SKU}
		if in.Brand != nil {
			o.Brand = *in.Brand
		}
		if in.Price != nil {
			o.Price = *in.Price
		}
		got := map[string]any{
			"parse":    gunsnprices.ParseTitle(in.Title, o),
			"model":    gunsnprices.IdentifyModel(in.Title, o),
			"find":     ids(gunsnprices.FindCalibers(in.Title)),
			"norm":     nil,
			"cap":      nil,
			"bbl":      nil,
			"rpb":      nil,
			"ammo":     gunsnprices.LooksLikeAmmo(in.Title),
			"unescape": pyre.Unescape(in.Title),
		}
		if c, ok := gunsnprices.NormalizeCaliber(in.Title); ok {
			got["norm"] = c.ID
		}
		if n, ok := gunsnprices.MagazineCapacity(in.Title); ok {
			got["cap"] = n
		}
		if v, ok := gunsnprices.BarrelLength(in.Title); ok {
			got["bbl"] = jsonFloat(v)
		}
		if n, ok := gunsnprices.RoundsPerBox(in.Title); ok {
			got["rpb"] = n
		}
		for k, want := range rec.Out {
			g, err := json.Marshal(got[k])
			if err != nil {
				t.Fatal(err)
			}
			if sameJSON(want, g) {
				continue
			}
			bad[k]++
			if shown++; shown <= 15 {
				t.Errorf("DIFF %s: %+v\n  python: %.600s\n  go:     %.600s", k, in, want, g)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k, n := range bad {
		keys = append(keys, fmt.Sprintf("%s=%d", k, n))
	}
	sort.Strings(keys)
	t.Logf("records: %d, differences: %d %v (%.1f s)", records, len(keys), keys, time.Since(started).Seconds())
}

func ids(cs []gunsnprices.Caliber) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

// jsonFloat writes a float like Python's json module ("26.0").
type jsonFloat float64

func (f jsonFloat) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(gunsnprices.ProductInfo{BarrelLength: (*float64)(&f)})
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m["barrel_length"], nil
}

// sameJSON compares two JSON documents strictly: object keys and order-free, numbers by value and by form (an
// integer is not equal to a float such as 26.0).
func sameJSON(a, b []byte) bool {
	var x, y any
	da := json.NewDecoder(bytes.NewReader(a))
	da.UseNumber()
	db := json.NewDecoder(bytes.NewReader(b))
	db.UseNumber()
	if da.Decode(&x) != nil || db.Decode(&y) != nil {
		return false
	}
	return sameValue(x, y)
}

func sameValue(x, y any) bool {
	switch a := x.(type) {
	case json.Number:
		b, ok := y.(json.Number)
		if !ok {
			return false
		}
		isFloat := func(n json.Number) bool { return strings.ContainsAny(string(n), ".eE") }
		if isFloat(a) != isFloat(b) {
			return false
		}
		fa, ea := a.Float64()
		fb, eb := b.Float64()
		return ea == nil && eb == nil && fa == fb
	case []any:
		b, ok := y.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !sameValue(a[i], b[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		b, ok := y.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for k, v := range a {
			w, ok := b[k]
			if !ok || !sameValue(v, w) {
				return false
			}
		}
		return true
	default:
		return x == y
	}
}
