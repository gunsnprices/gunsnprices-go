package gunsnprices

import (
	"math"
	"strconv"
	"strings"

	"github.com/gunsnprices/gunsnprices-go/internal/pyre"
)

func specPrep(text string) string {
	s := pyre.Lower(pyre.Unescape(pyre.Unescape(text)))
	for _, r := range [][2]string{{"”", `"`}, {"″", `"`}, {"“", `"`}, {"''", `"`}, {"’’", `"`}, {"®", ""}, {"™", ""}} {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return pyre.CollapseSpace(s)
}

var ammoRx = mustRx(`\b(?:ammo|ammunition|shotshells?|shot ?shells?|cartridges?|bullets?|primers?|slugs?|buckshot|pellets?|bbs?|grains?|\d+\s*gr|`+
	`per box|box of|rounds? box|rds?/box)\b|/\s*(?:box|bx|pk|pack|case|cs|tin)\b`, false)

// LooksLikeAmmo reports whether a title reads like ammunition (bullet weight, "ammo", "20/box", ...) rather than a
// firearm.
func LooksLikeAmmo(title string) bool {
	return ammoRx.IsMatch([]rune(specPrep(clean(title))))
}

// ---------- magazine capacity ----------

var (
	capPlus = mustRx(`(?<![\d.:/x])(\d{1,3})\s*\+\s*1(?!\d)`, false) // 15+1
	capRd   = mustRx(`(?<![\d.:/x])(\d{1,3})\s*-?\s*(?:rd|rds|rnd|rnds|round|rounds|shot|shots)\b(?!\s*/?\s*(?:per\b|box|bx|pack|pk|case))`, false)
	// "13rd/15rd", "1-30Rd/1-10Rd Mag": ships with magazines of two sizes - take the larger one
	capPair = mustRx(`(?<![\d.:/x])(\d{1,3})\s*-?\s*(?:rd|rds|rnd|rnds|rounds?)?\s*/\s*(?:1?-)?(\d{1,3})\s*-?\s*(?:rd|rds|rnd|rnds|rounds?)\b`, false)
	capMag  = mustRx(`\b(?:cap|capacity)\s*[:=]?\s*(\d{1,3})\b(?!\s*(?:\"|in\b|gr\b))`, false)
)

func groupInt(m *pyre.Match, i int) int {
	n, _ := pyre.Int(m.Group(i))
	return n
}

// MagazineCapacity returns the magazine capacity of a firearm or magazine from its title ("17RD", "15+1",
// "10-shot"). For "15+1" it is 15. Use it on firearm and magazine titles: in ammunition titles "20 rounds" means
// rounds per box (see RoundsPerBox).
func MagazineCapacity(title string) (int, bool) {
	s := []rune(specPrep(clean(title)))
	if m := capPair.Search(s, 0); m != nil {
		if n := max(groupInt(m, 1), groupInt(m, 2)); n >= 1 && n <= 110 {
			return n, true
		}
	}
	for _, rx := range []*pyre.Regexp{capPlus, capRd, capMag} {
		for _, m := range rx.FindAll(s) {
			if n := groupInt(m, 1); n >= 1 && n <= 110 {
				return n, true
			}
		}
	}
	return 0, false
}

// ---------- barrel length ----------

const (
	inchRx = `(?:"|in\b(?!\s*(?:1\b|stock\b|one\b))\.?|inch(?:es)?\b|-inch\b|-in\b)`
	lenRx  = `(\d{1,2}(?:\.\d{1,3})?)(?:[\s-]+(\d{1,2})/(\d{1,2}))?` // 18.5 / 16 1/2
	bblRx  = `(?:bbl|brl|barrel)\b`
)

var (
	barTagged = []*pyre.Regexp{
		mustRx(`(?<![\d.:/x#$-])`+lenRx+`\s*-?\s*`+inchRx+`?\s*`+bblRx, false),            // 20" barrel, 22 bbl
		mustRx(`\b(?:bbl|brl|barrel)\s*(?:length)?\s*[:=]?\s*`+lenRx+`\s*`+inchRx, false), // barrel: 20"
	}
	barInch         = mustRx(`(?<![\d.:/x#$-])`+lenRx+`\s*-?\s*`+inchRx, false)
	notBarrelAfter  = mustRx(`\s*-?\s*(?:chamber|chbr|chmbr|shells?|m-?lok|mlok|handguard|hg\b|rail|keymod|quad|lop\b|length of pull|oal\b|twist|rate)`, false)
	notBarrelBefore = mustRx(`(?:lop|oal|overall|length of pull|pull|twist|1:|chamber|m-?lok|mlok|handguard|rail|sight radius)\s*[:=]?\s*$`, false)
	shotgunRx       = mustRx(`\b(?:\d{1,2}|410)\s*(?:ga|gauge|g)\b|\.410|shotgun|o/u|over/under|sxs|side by side|\b(?:12|16|20|28|410)/\d\d\b`, false)
	// "O/U 12/28": gauge / barrel length (10 is left out: "10/22" is a Ruger rifle)
	gaugeSlash = mustRx(`(?<![a-z0-9./$#-])(?:12|16|20|28|410)/(1[89](?:\.5)?|2\d|3[0-4])(?![\d/.])(?!\s*(?:ga|gauge|rd|round))`, false)
)

// floatOf is Python's float() of a number matched by \d (digits of any script).
func floatOf(s string) float64 {
	f, err := strconv.ParseFloat(pyre.ASCIIDigits(s), 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

func barLength(m *pyre.Match) float64 {
	v := floatOf(m.Group(1))
	if m.Group(2) != "" && m.Group(3) != "" {
		num, den := groupInt(m, 2), groupInt(m, 3)
		if (den == 2 || den == 4 || den == 8 || den == 16) && num < den {
			v += float64(num) / float64(den)
		}
	}
	return v
}

// BarrelLength returns the barrel length in inches from a firearm title ('4.25"', "16 in", "20-inch", '16 1/2"',
// "22 BBL"). Chamber length (3" on shotguns), twist rate, handguard and length-of-pull values are skipped.
func BarrelLength(title string) (float64, bool) {
	s := []rune(specPrep(clean(title)))
	for _, rx := range barTagged {
		for _, m := range rx.FindAll(s) {
			if v := barLength(m); v >= 1.5 && v <= 36 {
				return v, true
			}
		}
	}
	if m := gaugeSlash.Search(s, 0); m != nil {
		return floatOf(m.Group(1)), true
	}
	shotgun := shotgunRx.IsMatch(s)
	for _, m := range barInch.FindAll(s) {
		v := barLength(m)
		if !(v >= 1.5 && v <= 36) {
			continue
		}
		if notBarrelAfter.MatchAt(s, m.End()) != nil || notBarrelBefore.IsMatch(s[max(0, m.Start()-20):m.Start()]) {
			continue
		}
		if shotgun && v <= 3.5 { // 3" chamber
			continue
		}
		return v, true
	}
	return 0, false
}

// ---------- rounds per box ----------

const (
	numRx  = `(\d{1,3}(?:,\d{3})+|\d{1,5})`
	rpbPre = `(?<![a-z0-9.#$/])`
)

var (
	rpbRx = []*pyre.Regexp{
		mustRx(rpbPre+numRx+`\s*-?\s*(?:rd|rds|round|rounds|rnd|rnds|ct|count|pcs|pc|cartridges|shells|shotshells|bullets)\b(?!\s*(?:mag|magazine|capacity|cap)\b)`, false),
		mustRx(rpbPre+numRx+`\s*(?:/\s*|per\s+)(?:box|bx|pack|pk|case|cs|tin|can|bag|ct)\b`, false),
		mustRx(`(?:box|pack|case|tin|bx|bag|can) of\s*`+numRx, false),
		mustRx(rpbPre+numRx+`\s*-?\s*(?:pack|pk|packs)\b`, false),
	}
	// distributor notation "50/10", "20/25": rounds per box / boxes per case
	boxCase      = mustRx(`(?<![\d.\-/])(5|10|15|18|20|25|30|40|50|100|200|250|500|1000)\s*/\s*(\d{1,3})(?![\d/.\"])`, false)
	caliberSlash = map[[2]string]bool{
		{"30", "30"}: true, {"45", "70"}: true, {"30", "06"}: true, {"22", "250"}: true, {"25", "06"}: true,
		{"38", "55"}: true, {"44", "40"}: true, {"32", "20"}: true, {"38", "40"}: true, {"25", "20"}: true,
		{"32", "40"}: true,
	}
)

// RoundsPerBox returns the number of rounds in one package of ammunition from its title ("20rds", "50/box",
// "Box of 25", "1,000 Rounds").
func RoundsPerBox(title string) (int, bool) {
	s := []rune(specPrep(clean(title)))
	for _, rx := range rpbRx {
		if m := rx.Search(s, 0); m != nil {
			n, _ := pyre.Int(strings.ReplaceAll(m.Group(1), ",", ""))
			if n >= 1 && n <= 20000 {
				return n, true
			}
		}
	}
	for _, m := range boxCase.FindAll(s) {
		a, b := m.Group(1), m.Group(2)
		if !caliberSlash[[2]string{a, b}] && b != "2" && b != "4" && b != "8" && b != "16" && b != "32" {
			n, _ := pyre.Int(a)
			return n, true
		}
	}
	return 0, false
}

// CostPerRound returns the price of one round, rounded to 4 decimals like Python's round(). It reports false
// without a positive price or round count.
func CostPerRound(price float64, rounds int) (float64, bool) {
	if price == 0 || rounds <= 0 || price <= 0 {
		if !math.IsNaN(price) || rounds <= 0 {
			return 0, false
		}
	}
	return round4(price / float64(rounds)), true
}

// round4 is Python's round(x, 4): the exact binary value rounded half to even, then the nearest float.
func round4(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	f, err := strconv.ParseFloat(strconv.FormatFloat(x, 'f', 4, 64), 64)
	if err != nil {
		return x
	}
	if f == 0 && math.Signbit(x) {
		return math.Copysign(0, -1)
	}
	return f
}
