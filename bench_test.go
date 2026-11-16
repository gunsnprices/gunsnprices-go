package gunsnprices_test

import (
	"testing"

	"github.com/gunsnprices/gunsnprices-go"
)

// BenchmarkParseTitle parses the 1,150 golden titles (one op = one title).
func BenchmarkParseTitle(b *testing.B) {
	cases := loadGolden(&testing.T{})
	gunsnprices.ParseTitle("warm up", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := cases[i%len(cases)]
		gunsnprices.ParseTitle(c.Args.Title, optionsOf(c))
	}
}
