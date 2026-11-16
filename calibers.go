package gunsnprices

import (
	"sort"
	"strings"
	"sync"

	"github.com/gunsnprices/gunsnprices-go/internal/pyre"
)

// Caliber is a caliber (cartridge) from the GunsNPrices caliber list.
type Caliber struct {
	// ID is a stable identifier, such as "9x19mmparabellum".
	ID string `json:"id"`
	// Name is the display name, such as "9×19mm Parabellum".
	Name string `json:"name"`
	// Type is "handgun", "rifle", "rimfire", "shotgun", "airgun" or "muzzleloader".
	Type string `json:"type"`
	// URL is the GunsNPrices page that compares prices for this caliber ("" if there is none).
	URL string `json:"url"`
}

// word -> spellings used in titles; a word marked optional may be left out ("300 Blackout" for ".300 AAC Blackout")
var calSynonyms = map[string][]string{
	"win": {"win", "winchester"}, "winchester": {"winchester", "win"},
	"rem": {"rem", "remington"}, "remington": {"remington", "rem"},
	"mag": {"mag", "magnum"}, "magnum": {"magnum", "mag"},
	"wby": {"wby", "weatherby"}, "weatherby": {"weatherby", "wby"},
	"spl": {"spl", "special"}, "special": {"special", "spl"},
	"sprg": {"sprg", "springfield", "spfd"}, "springfield": {"springfield", "sprg", "spfd"},
	"government": {"government", "govt", "gov"},
	"acp":        {"acp", "auto"}, "auto": {"auto", "acp"},
	"s&w":        {"s&w", "s & w", "sw", "smith & wesson", "smith and wesson", "smith&wesson"},
	"blackout":   {"blackout", "blk", "black out"},
	"creedmoor":  {"creedmoor", "creed", "cm", "crd", "creedmore"},
	"parabellum": {"parabellum", "para", "luger"},
	"gauge":      {"gauge", "ga", "gauges"},
	"bore":       {"bore", "ga", "gauge"},
	"lr":         {"lr", "long rifle"},
	"h&h":        {"h&h", "h & h", "holland & holland", "holland and holland"},
	"h&r":        {"h&r", "h & r"},
	"bm":         {"bm", "bushmaster", "bush"},
}

var calOptional = map[string]bool{
	"aac": true, "nato": true, "win": true, "winchester": true, "rem": true, "remington": true, "sprg": true,
	"springfield": true, "government": true,
}

const calSep = `[\s\-]*`

// common title spellings that the synonym table does not produce
var calExtra = map[string][]string{
	".300 AAC Blackout":    {"300 aac", "300 bo", "300 black out", "300 blackout", "300 blk"},
	"6.5mm Creedmoor":      {"6.5 creedmoor", "6.5 crd", "6.5 cm", "6.5mm cm", "6.5 cr"},
	"6mm Creedmoor":        {"6 creedmoor"},
	".22 LR":               {"22lr", "22 lr", "22 long rifle"},
	".22 WMR":              {"22 mag", "22 magnum", "22 win mag", "22 wmr"},
	"5.56×45mm NATO":       {"5.56 nato", "5.56mm nato", "556 nato", "5.56x45", "5.56", "5.56mm", "5.56 mm"},
	"7.62x51mm NATO":       {"7.62 nato", "7.62mm nato", "762 nato", "762x51"},
	"7.62x39mm":            {"762x39"},
	".410 Bore":            {"410 ga", "410 gauge", "410 bore"},
	".45 Colt":             {"45 lc", "45 long colt"},
	".44 S&W Special":      {"44 spl", "44 special", "44 spcl"},
	".38 S&W Special":      {"38 spl", "38 special", "38 spcl", "38 spc", "38 sp", "38 spp", "38+p"},
	".357 Mag":             {"357 rm", "357 rem mag", "357 rem magnum", "357 remington magnum", "357 remington mag"},
	"9×19mm Parabellum":    {"9mm luger", "9mm para", "9 mm luger", "9mm"},
	"10mm Auto":            {"10mm", "10 mm"},
	".45 ACP":              {"45 acp", "45 auto", "45 automatic", "45 automatic colt pistol"},
	".40 S&W":              {"40 s&w", "40 sw", "40 cal", "40s", ".40"},
	".380 ACP":             {"380 acp", "380 auto", "380"},
	".243 Win":             {"243", ".243"},
	".308 Win":             {"308", ".308"},
	".350 Legend":          {".350", "350 leg", "350 lgnd", "350 lgd"},
	".450 BM":              {"450 bushmaster", "450 bush"},
	".204 Ruger":           {".204"},
	".177":                 {"177", ".177"},
	".300 Win Mag":         {"300 wm"},
	"7mm Remington Magnum": {"7mm rm", "7 rm", "7mm rem mag"},
	".338 Lapua Mag":       {"338 lapua", "338 lap"},
	".30 Carbine":          {"30 m1", "30 m1 carbine"},
	".30-06 Springfield":   {"30-06 sp"},
	"7.92×57mm Mauser":     {"8x57", "8x57mm", "8x57 js", "8x57 is"},
	".50 AE":               {"50 action express"},
	"12 Gauge":             {"12 ga", "12ga", "12 gauge", "12-gauge", "12g"},
	"20 Gauge":             {"20 ga", "20ga", "20 gauge", "20-gauge", "20g"},
	"16 Gauge":             {"16 ga", "16ga", "16 gauge", "16g"},
	"28 Gauge":             {"28 ga", "28ga", "28 gauge", "28g"},
	"10 Gauge":             {"10g"},
}

// a bare number or "Nmm" ("20", "307", "6.5mm") in a title is usually something else, not a caliber;
// the exceptions below were checked against GunsNPrices listings (right in 97%+ of cases)
var (
	calBare      = mustRx(`^\.?\d+(?:\.\d+)?(?:\s*mm)?$`, false)
	calAllowBare = map[string]bool{
		"380": true, "410": true, "9mm": true, "10mm": true, "10 mm": true, "5.56": true, "5.56mm": true,
		"5.56 mm": true, "243": true, ".243": true, "308": true, ".308": true, ".40": true, ".350": true, ".204": true,
		"177": true, ".177": true,
	}
	// "12g" next to CO2 is a 12-gram CO2 cartridge, not 12 gauge
	calGram = mustRx(`^\d+\s?g$`, false)
	// shotguns "O/U 12/28": gauge / barrel length; on ammo "20/25" is rounds per box / boxes per case; "20/28 gauge" is
	// a two-gauge set
	calContext = []struct {
		rx *pyre.Regexp
		id string
	}{
		{mustRx(`(?<![a-z0-9./$#-])12/(?:1[89]|2\d|3[0-4])(?:\.5)?(?![\d/.])(?!\s*(?:ga|gauge|rd|round))`, false), "12gauge"},
		{mustRx(`(?<![a-z0-9./$#-])20/(?:18|18\.5|2[0-46-9]|3[0-2])(?![\d/.])(?!\s*(?:ga|gauge|rd|round))`, false), "20gauge"},
		{mustRx(`(?<![a-z0-9./$#-])28/(?:2[6-9]|3[0-2])(?![\d/.])(?!\s*(?:ga|gauge|rd|round))`, false), "28gauge"},
		{mustRx(`(?<![a-z0-9.$#-])\.?357(?=\s*/\s*\.?38)`, false), "357mag"}, // ".357/.38 Spl"
	}

	calKeyDot   = mustRx(`(?<=\d)\.(?=\d)`, false)
	calKeyStrip = mustRx(`[^a-z0-9#&]+`, false)
	calMMx      = mustRx(`\.?\d+(?:\.\d+)?x\d+mm[a-z]*`, false)
	calNmm      = mustRx(`\d+mm`, false)
	calXDigits  = mustRx(`(?<=\d)x(?=\d)`, false)
	calXTmpl    = mustTemplate(`\\s*x\\s*`, calXDigits)
	calDigit    = mustRx(`\d`, false)
)

// cyrillicHa is the Cyrillic letter that looks like "x" (U+0445).
const cyrillicHa = string(rune(0x0445))

func calKey(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(pyre.Lower(pyre.Unescape(s)), "×", "x"), cyrillicHa, "x")
	s = sub(calKeyDot, s, hashTemplate)
	s = sub(calKeyStrip, s, emptyTemplate)
	return strings.ReplaceAll(strings.ReplaceAll(s, "#", "."), "&", "and")
}

func calTokens(name string) []string {
	s := strings.ReplaceAll(pyre.Lower(pyre.Unescape(name)), "×", "x")
	s = strings.TrimPrefix(s, ".")
	return pyre.Fields(s)
}

func calTokenAlts(tok string) []string {
	r := []rune(tok)
	if calMMx.FullMatch(r) != nil { // 9x19mm, 5.56x45mm, 7.62x54mmr
		return []string{tok, strings.ReplaceAll(tok, "mm", "")}
	}
	if calNmm.FullMatch(r) != nil {
		return []string{tok, tok[:len(tok)-2] + " mm"}
	}
	alts := []string{tok}
	if syn, ok := calSynonyms[tok]; ok {
		alts = append([]string(nil), syn...)
	}
	if calOptional[tok] {
		alts = append(alts, "")
	}
	return alts
}

func isBare(s string) bool { return calBare.MatchAt([]rune(s), 0) != nil }

func calVariants(name string) []string {
	toks := calTokens(name)
	if len(toks) == 0 || strings.Contains(name, "/") {
		return nil
	}
	alts := make([][]string, len(toks))
	for i, t := range toks {
		alts[i] = calTokenAlts(t)
	}
	seen := map[string]bool{}
	// itertools.islice(itertools.product(*alts), 400): the last token varies fastest
	idx := make([]int, len(toks))
	parts := make([]string, 0, len(toks))
	for n := 0; n < 400; n++ {
		parts = parts[:0]
		for i := range toks {
			if x := alts[i][idx[i]]; x != "" {
				parts = append(parts, x)
			}
		}
		if v := strings.Join(parts, " "); v != "" && (!isBare(v) || calAllowBare[v]) {
			seen[v] = true
		}
		k := len(idx) - 1
		for ; k >= 0; k-- {
			idx[k]++
			if idx[k] < len(alts[k]) {
				break
			}
			idx[k] = 0
		}
		if k < 0 {
			break
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func calVariantRx(variant string) string {
	toks := strings.Split(variant, " ")
	parts := make([]string, len(toks))
	for i, tok := range toks {
		t := pyre.Escape(tok)
		t = sub(calXDigits, t, calXTmpl)               // 7.62 x 39
		parts[i] = strings.ReplaceAll(t, `\-`, calSep) // 30-06 / 30 06
	}
	body := strings.Join(parts, calSep)
	if r := []rune(variant); len(r) > 0 && pyre.IsDigit(r[0]) {
		return `\.?` + body
	}
	return body
}

type calIndex struct {
	byKey map[string]Caliber
	big   *pyre.Regexp
	byID  map[string]Caliber
	list  []Caliber // in file order
}

var (
	calOnce sync.Once
	calIdx  *calIndex
)

func calibers() *calIndex {
	calOnce.Do(func() { calIdx = buildCalibers() })
	return calIdx
}

func buildCalibers() *calIndex {
	root := mustObject(calibersJSON, "calibers.json")
	type cand struct {
		n   int
		cal Caliber
	}
	idx := &calIndex{byKey: map[string]Caliber{}, byID: map[string]Caliber{}}
	conflict := map[string][]cand{}
	var order []string // keys of conflict in insertion order
	pats := map[string]string{}
	for _, item := range root.list("calibers") {
		c, _ := item.(*object)
		cal := Caliber{ID: c.str("id"), Name: c.str("name"), Type: c.str("type"), URL: c.str("url")}
		if _, dup := idx.byID[cal.ID]; !dup {
			idx.list = append(idx.list, cal)
		} else {
			for i := range idx.list {
				if idx.list[i].ID == cal.ID {
					idx.list[i] = cal
				}
			}
		}
		idx.byID[cal.ID] = cal
		names := append([]string{c.str("name"), c.str("site_name")}, c.strings("aliases")...)
		names = append(names, calExtra[cal.Name]...)
		for _, n := range names {
			own := pyre.Lower(pyre.Unescape(n))
			vs := calVariants(n)
			if !isBare(own) || calAllowBare[own] {
				vs = append(vs, own)
			}
			for _, v := range vs {
				k := calKey(v)
				if k == "" || !calDigit.MatchString(k) {
					continue
				}
				cs, ok := conflict[k]
				if !ok {
					order = append(order, k)
				}
				known := false
				for _, x := range cs {
					known = known || x.cal.ID == cal.ID
				}
				if !known {
					cs = append(cs, cand{number(c.get("n")), cal})
				}
				conflict[k] = cs
				if _, ok := pats[k]; !ok {
					if v != own || !strings.Contains(v, "/") {
						pats[k] = calVariantRx(v)
					} else {
						pats[k] = pyre.Escape(v)
					}
				}
			}
		}
	}
	var keys []string
	for _, k := range order {
		cs := conflict[k]
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].n > cs[j].n })
		// one spelling, several calibers: keep the one with 10x+ more listings, otherwise the spelling is ambiguous
		if len(cs) == 1 || cs[0].n >= 10*max(1, cs[1].n) {
			idx.byKey[k] = cs[0].cal
			keys = append(keys, k)
		}
	}
	sort.SliceStable(keys, func(i, j int) bool { return runeLen(pats[keys[i]]) > runeLen(pats[keys[j]]) })
	alts := make([]string, len(keys))
	for i, k := range keys {
		alts[i] = pats[k]
	}
	idx.big = mustRx(`(?<![a-z0-9.$])(?:`+strings.Join(alts, "|")+`)(?![a-z0-9])`, true)
	return idx
}

func calPrep(text string) string {
	s := pyre.Lower(pyre.Unescape(text))
	s = strings.ReplaceAll(s, "×", "x")
	s = strings.ReplaceAll(s, "”", `"`)
	s = strings.ReplaceAll(s, "″", `"`)
	s = strings.ReplaceAll(s, "®", "")
	s = strings.ReplaceAll(s, "™", "")
	return pyre.CollapseSpace(s)
}

// FindCalibers returns every caliber mentioned in text, in order of appearance, without duplicates.
//
// Besides the caliber names and the retailer spellings in the caliber list, it reads common title forms such as
// "300 Blackout", "12ga", "7.62 x 39", "45 Auto", "6.5 Creed" and ".308 Winchester", and the shotgun notation
// "O/U 12/28" (12 gauge, 28" barrels).
func FindCalibers(text string) []Caliber {
	idx := calibers()
	s := []rune(calPrep(clean(text)))
	co2 := strings.Contains(string(s), "co2")
	type hit struct {
		pos int
		cal Caliber
	}
	var hits []hit
	for _, m := range idx.big.FindAll(s) {
		g := m.Group(0)
		if co2 && calGram.MatchAt([]rune(g), 0) != nil {
			continue
		}
		if c, ok := idx.byKey[calKey(g)]; ok {
			hits = append(hits, hit{m.Start(), c})
		}
	}
	for _, cx := range calContext {
		for _, m := range cx.rx.FindAll(s) {
			hits = append(hits, hit{m.Start(), idx.byID[cx.id]})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })
	out := []Caliber{}
	seen := map[string]bool{}
	for _, h := range hits {
		if !seen[h.cal.ID] {
			seen[h.cal.ID] = true
			out = append(out, h.cal)
		}
	}
	return out
}

// FindCaliber returns the first caliber mentioned in text.
func FindCaliber(text string) (Caliber, bool) {
	found := FindCalibers(text)
	if len(found) == 0 {
		return Caliber{}, false
	}
	return found[0], true
}

// NormalizeCaliber maps a caliber field from a product feed ("9mm Luger", "5.56 NATO", "12 GA",
// "308 WIN (7.62X51 NATO)") to a Caliber. The whole value is looked up first; if it is not a known spelling, the first
// caliber found inside it is returned.
func NormalizeCaliber(value string) (Caliber, bool) {
	value = clean(value)
	if c, ok := calibers().byKey[calKey(value)]; ok {
		return c, true
	}
	return FindCaliber(value)
}

// CaliberByID looks up a caliber by its ID ("9x19mmparabellum").
func CaliberByID(id string) (Caliber, bool) {
	c, ok := calibers().byID[id]
	return c, ok
}

// ListCalibers returns the full caliber list, sorted by name.
func ListCalibers() []Caliber {
	out := append([]Caliber(nil), calibers().list...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
