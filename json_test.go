package gunsnprices

import (
	"math"
	"testing"

	"github.com/gunsnprices/gunsnprices-go/internal/pyre"
)

// pyRepr must format floats like Python's repr() (checked with Python 3.13).
func TestPyRepr(t *testing.T) {
	cases := map[float64]string{
		26.0: "26.0", 0.45: "0.45", 1e-05: "1e-05", 0.0001: "0.0001", 1e16: "1e+16", 1e15: "1000000000000000.0",
		123456789012345.6: "123456789012345.6", 1.5e300: "1.5e+300", 0.30000000000000004: "0.30000000000000004",
		5e-324: "5e-324", 2.5e-5: "2.5e-05", 4.02: "4.02", math.Copysign(0, -1): "-0.0",
	}
	for x, want := range cases {
		if got := pyRepr(x); got != want {
			t.Errorf("pyRepr(%v) = %q, want %q", x, got, want)
		}
	}
}

func TestUnicodeVersion(t *testing.T) {
	if UnicodeVersion != pyre.UnicodeVersion {
		t.Errorf("UnicodeVersion %q, tables %q", UnicodeVersion, pyre.UnicodeVersion)
	}
}
