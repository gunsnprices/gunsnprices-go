package gunsnprices

import (
	"strings"
	"sync"

	"github.com/gunsnprices/gunsnprices-go/internal/pyre"
)

// ModelMatch is the result of IdentifyModel.
type ModelMatch struct {
	// Brand is the brand as named on GunsNPrices, such as "Glock".
	Brand string `json:"brand"`
	// Family is the model family, such as "Glock 19".
	Family string `json:"family"`
	// Model is the most specific model, such as "Glock 19 Gen 5 MOS".
	Model string `json:"model"`
	// Generation is "5", "V", "V3", ... or "" when unknown.
	Generation string `json:"generation"`
	// Configs are the configurations found, such as ["MOS"] or ["Compact"].
	Configs []string `json:"configs"`
	// Features are the features found, such as ["Threaded Barrel", "FDE"].
	Features []string `json:"features"`
	// Source is "title", or "mpn" when the model was decoded from a Glock part number.
	Source string `json:"source"`
}

// airguns and airsoft are not firearms (the site filters them by category; here we go by the title)
var airgunRx = mustRx(`\bair\s*-?\s*(?:gun|rifle|pistol)s?\b|\bairgun|\bairsoft|\bbb\s*(?:gun|pistol|rifle)|\bpellets?\b|\bco2\b`, true)

// accessories named after a gun ("Glock 19 holster", "Sig P365 XL 12rd Magazine", "Magpul PMAG for Glock 19"). A title
// that describes a gun (pistol/rifle/..., "15+1", a barrel length) or mentions the item as included ("w/ 3 magazines",
// "no magazine") is not an accessory.
var (
	accNoun = mustRx(`\b(?:holsters?|magazines?(?!\s*(?:pattern|compatible|well|release|disconnect|safety|catch|cut|tube|extension|fed)\b)|pmags?|slings?|`+
		`grip panels?|scope mounts?|recoil pads?|speed ?loaders?|mag(?:azine)? loaders?|cleaning kits?|pouch(?:es)?|snap caps?|replacement barrels?|`+
		`conversion kits?|parts kits?)\b`, false)
	accIncluded = mustRx(`(?:(?:\bw/|\bwith\b|\bwithout\b|\bno\b|\band\b|&|\+|\bincl\w*|\bplus\b|\(\d\)|\b[1-9]\b|\btwo\b|\bthree\b|\bfour\b|,|\bor\b|\bextra\b|`+
		`\bspare\b|\bfactory\b|\bfits\b|\btakes\b|\baccepts\b|\buses\b)[^,]{0,14}|/\s*)$`, false)
	gunWords = mustRx(`\b(?:pistol|rifle|shotgun|revolver|handgun|carbine|firearm|semi-?auto(?:matic)?|bolt[- ]?action|pump|lever[- ]?action|single[- ]?shot|`+
		`o/u|over/under|sxs|sbr|derringer)\b|\d+\+1\b|\d+(?:\.\d+)?\s*-?\s*(?:\"|in\b|inch)`, false)
)

func isAccessory(title string) bool {
	t := []rune(pyre.Lower(pyre.Unescape(pyre.Unescape(title))))
	if gunWords.IsMatch(t) {
		return false
	}
	for _, m := range accNoun.FindAll(t) {
		if !accIncluded.IsMatch(t[max(0, m.Start()-16):m.Start()]) {
			return true
		}
	}
	return false
}

// dashes is str.maketrans of the Python package: dashes and quotes to ASCII, "|" and "*" spaced, marks removed.
var dashes = map[rune]string{
	0x2010: "-", 0x2011: "-", 0x2013: "-", 0x2014: "-", 0x201C: `"`, 0x201D: `"`, 0x2019: "'",
	'|': " | ", '*': " ", 0xAE: "", 0x2122: "",
}

func translateDashes(s string) string {
	for i, r := range s {
		if _, ok := dashes[r]; !ok {
			continue
		}
		var b strings.Builder
		b.WriteString(s[:i])
		for _, r := range s[i:] {
			if v, ok := dashes[r]; ok {
				b.WriteString(v)
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	return s
}

var keyStripRx = mustRx(`[^a-z0-9]+`, false)

// modelKey is the models module's _key: lowercase letters and digits only.
func modelKey(s string) string { return sub(keyStripRx, pyre.Lower(pyre.Unescape(s)), emptyTemplate) }

type namedRx struct {
	name string
	rx   *pyre.Regexp
}

type rewriteRule struct {
	rx   *pyre.Regexp
	tmpl *pyre.Template
}

type termModel struct {
	name, family   string
	match, not     *pyre.Regexp
	configs        map[string]bool
	impliedConfigs map[string]bool
}

type modelDict struct {
	brand, brandWord string
	terms            bool
	rewrite          []rewriteRule
	exclude          *pyre.Regexp
	excludeSoft      *pyre.Regexp
	block            *pyre.Regexp
	brandAny         *pyre.Regexp
	brandStart       *pyre.Regexp
	configs          []namedRx
	features         []namedRx
	gens             []namedRx
	brandNames       map[string]bool
	denyNames        map[string]bool
	featureOrder     []string
	needed           map[string][]string
	familyDisplay    map[string]string
	leafRx           map[string]*pyre.Regexp // config name -> the name as a word

	// terms mode
	termModels       []termModel
	otherBrandModels map[string]bool // nil: any model
	configImplied    map[string]map[string][]string

	// numeric mode (Glock)
	models         map[string]string
	findRx         *pyre.Regexp
	ownStartRx     *pyre.Regexp
	bareRx         *pyre.Regexp
	mpnModelAlias  map[string]string
	mpnLetterModel map[string]string
	mpnLetterGen   map[string]string
	noGenModels    map[string]bool
}

func anyRx(patterns []string) *pyre.Regexp {
	if len(patterns) == 0 {
		return nil
	}
	return mustRx(strings.Join(patterns, "|"), true)
}

func namedRxs(o *object, key string) []namedRx {
	var out []namedRx
	for _, p := range o.pairs(key) {
		out = append(out, namedRx{p[0], mustRx(p[1], true)})
	}
	return out
}

func stringSet(l []string) map[string]bool {
	m := make(map[string]bool, len(l))
	for _, s := range l {
		m[s] = true
	}
	return m
}

func keySet(l []string) map[string]bool {
	m := make(map[string]bool, len(l))
	for _, s := range l {
		m[modelKey(s)] = true
	}
	return m
}

var (
	modelsOnce sync.Once
	modelDicts []*modelDict
)

func dicts() []*modelDict {
	modelsOnce.Do(func() { modelDicts = loadModels() })
	return modelDicts
}

func loadModels() []*modelDict {
	root := mustObject(modelsJSON, "models.json")
	var out []*modelDict
	for _, item := range root.list("dicts") {
		o, _ := item.(*object)
		d := &modelDict{
			brand:         o.str("brand"),
			brandWord:     o.str("brand_word"),
			terms:         o.str("mode") == "terms",
			exclude:       anyRx(o.strings("exclude")),
			excludeSoft:   anyRx(o.strings("exclude_soft")),
			block:         anyRx(o.strings("other_brand_block")),
			configs:       namedRxs(o, "configs"),
			features:      namedRxs(o, "features"),
			gens:          namedRxs(o, "gens"),
			brandNames:    keySet(o.strings("brand_names")),
			denyNames:     keySet(o.strings("deny_brand_names")),
			featureOrder:  o.strings("feature_order"),
			familyDisplay: o.stringMap("family_display"),
			needed:        map[string][]string{},
			leafRx:        map[string]*pyre.Regexp{},
		}
		for _, p := range o.pairs("title_rewrite") {
			rx := mustRx(p[0], true)
			d.rewrite = append(d.rewrite, rewriteRule{rx, mustTemplate(p[1], rx)})
		}
		if brandRx := o.str("brand_rx"); brandRx != "" {
			d.brandAny = mustRx(brandRx, true)
			d.brandStart = mustRx(`^\W*(?:`+brandRx+`)`, true)
		} else {
			d.brandAny = mustRx(`(?<![a-z])`+pyre.Escape(d.brandWord)+`(?![a-z])`, true)
		}
		if needed := o.obj("needed"); needed != nil {
			for _, k := range needed.keys {
				d.needed[k] = needed.strings(k)
			}
		}
		for _, c := range d.configs {
			d.leafRx[c.name] = mustRx(`(?<![a-z0-9])`+pyre.Escape(c.name)+`(?![a-z0-9])`, true)
		}
		if d.terms {
			for _, mi := range o.list("models") {
				m, _ := mi.(*object)
				tm := termModel{
					name: m.str("name"), family: m.str("family"), match: mustRx(m.str("match"), true),
					configs: stringSet(m.strings("configs")), impliedConfigs: stringSet(m.strings("implied_configs")),
				}
				if not := m.str("not"); not != "" {
					tm.not = mustRx(not, true)
				}
				d.termModels = append(d.termModels, tm)
			}
			if allowed := o.strings("other_brand_models"); len(allowed) > 0 {
				d.otherBrandModels = stringSet(allowed)
			}
			d.configImplied = map[string]map[string][]string{}
			if imp := o.obj("config_implied_by_gen_family"); imp != nil {
				for _, name := range imp.keys {
					fam := map[string][]string{}
					if f := imp.obj(name); f != nil {
						for _, k := range f.keys {
							fam[k] = f.strings(k)
						}
					}
					d.configImplied[name] = fam
				}
			}
		} else {
			d.models = o.stringMap("models")
			bw := pyre.Escape(d.brandWord)
			d.findRx = mustRx(`(?<![a-z0-9.])(?:`+bw+`\s*-?\s*g?\s*-?\s*|g)(\d{2})(?:(sf|x|c|l|m|s)|\s(sf|x)(?![a-z0-9]))?(?:g([1-6])(?:us)?|(v))?(fs|mos|rtf)?`+
				`(?![a-z0-9&\"+/])(?!\s*-?\s*(?:round|rounds|rd|rds|rnd|mag|mags|magazine)\b)`, true)
			d.ownStartRx = mustRx(`^\W*`+bw+`\b`, true)
			if tokens := o.strings("bare_model_tokens"); len(tokens) > 0 {
				d.bareRx = mustRx(`\b(`+strings.Join(tokens, "|")+`)\b`, false)
			}
			d.mpnModelAlias = o.stringMap("mpn_model_alias")
			d.mpnLetterModel = o.stringMap("mpn_letter_model")
			d.mpnLetterGen = o.stringMap("mpn_letter_gen")
			d.noGenModels = stringSet(o.strings("no_gen_models"))
		}
		out = append(out, d)
	}
	return out
}

// ListBrands returns the brands covered by the model dictionaries.
func ListBrands() []string {
	ds := dicts()
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.brand
	}
	return out
}

// normBase is the part of the models module's _norm that does not depend on the dictionary.
func normBase(s string) string {
	return pyre.Strip(pyre.CollapseSpace(pyre.Lower(translateDashes(pyre.Unescape(s)))))
}

func (d *modelDict) norm(base string) string {
	s := base
	for _, rw := range d.rewrite {
		s = sub(rw.rx, s, rw.tmpl)
	}
	return s
}

// ---------- numeric mode (Glock): codes like 19, 19X, 43X, 20SF; generation from the title or the part number ----------

type numericModel struct {
	code, num string
	pos, end  int
	mos       bool
	gen       string
}

func (d *modelDict) findModels(t []rune) []numericModel {
	var found []numericModel
	for _, m := range d.findRx.FindAll(t) {
		num := pyre.Lower(m.Group(1))
		suf := m.Group(2)
		if !m.Has(2) {
			suf = m.Group(3)
		}
		suf = pyre.Upper(suf)
		code := ""
		if _, ok := d.models[num+suf]; ok {
			code = num + suf
		} else if _, ok := d.models[num]; ok {
			code = num
		}
		if code == "" {
			continue
		}
		gen := m.Group(4)
		if gen == "" && m.Has(5) {
			gen = "V"
		}
		found = append(found, numericModel{code: code, num: num, pos: m.Start(), end: m.End(),
			mos: pyre.Lower(m.Group(6)) == "mos", gen: gen})
	}
	return found
}

type decodedMPN struct {
	code, letter, mpn, gen string
}

var (
	mpnInTitle = mustRx(`\b(?:gl)?[pue][a-z0-9]\d{3}[a-z0-9-]*|\bg\d{2}c?\d{3}us\b|\bg\d{4}[a-z]*aut\b`, true)
	mpnUS      = mustRx(`^G(\d{2}C?)([345])(\d{2})US$`, true)
	mpnAUT     = mustRx(`^G(\d{2})\d{2}[A-Z]*AUT$`, true)
	mpnPUE     = mustRx(`^(?:GL)?([PUE])([A-Z0-9])(\d{2})(\d)`, true)
)

func (d *modelDict) decodeMPN(name, mpn, sku string) *decodedMPN {
	cands := append([]string{mpn, sku}, mpnInTitle.FindAllStrings([]rune(name))...)
	for _, c := range cands {
		c = pyre.Strip(c)
		rc := []rune(c)
		if m := mpnUS.MatchAt(rc, 0); m != nil {
			if code := pyre.Upper(m.Group(1)); hasKey(d.models, code) {
				return &decodedMPN{code: code, mpn: c, gen: m.Group(2)}
			}
		}
		if m := mpnAUT.MatchAt(rc, 0); m != nil && hasKey(d.models, m.Group(1)) {
			return &decodedMPN{code: m.Group(1), mpn: c}
		}
		m := mpnPUE.MatchAt(rc, 0)
		if m == nil {
			continue
		}
		letter, num := pyre.Upper(m.Group(2)), m.Group(3)
		if v, ok := d.mpnModelAlias[num]; ok {
			num = v
		}
		code := num
		if v, ok := d.mpnLetterModel[letter+num]; ok {
			code = v
		}
		if !hasKey(d.models, code) {
			continue
		}
		return &decodedMPN{code: code, letter: letter, mpn: c, gen: d.mpnLetterGen[letter]}
	}
	return nil
}

func hasKey(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}

var (
	leadingNumRx = mustRx(`^(\d{2})(x)?\b`, false)
	genRx        = mustRx(`\bgen(?:eration)?\.?\s*-?\s*([1-6])\b|\b([1-6])(?:st|nd|rd|th)\s*gen\b|\bg([1-6])\b`, false)
	vAfterRx     = mustRx(`^\s*v\b`, false)
	mpnTBRx      = mustRx(`\d(?:mos)?tb\b`, true)
)

type modelResult struct {
	code, family, gen string
	configs, features []string
	src               string
}

func (d *modelDict) parseNumeric(t []rune, name, brand, mpn, sku string) *modelResult {
	pm := d.decodeMPN(name, mpn, sku)
	if pm == nil && d.excludeSoft != nil && d.excludeSoft.IsMatch(t) {
		return nil
	}
	var own bool
	if brand != "" {
		own = d.brandNames[modelKey(brand)] || pm != nil
	} else {
		own = pm != nil || d.ownStartRx.MatchAt(t, 0) != nil
	}
	models := d.findModels(t)
	if len(models) == 0 && own {
		if m := leadingNumRx.MatchAt(t, 0); m != nil {
			if code := m.Group(1) + pyre.Upper(m.Group(2)); hasKey(d.models, code) {
				models = []numericModel{{code: code, num: m.Group(1), pos: 0, end: m.End()}}
			}
		}
	}
	if len(models) == 0 && own && d.bareRx != nil {
		if m := d.bareRx.Search(t, 0); m != nil {
			g := []rune(m.Group(1))
			models = []numericModel{{code: pyre.Upper(string(g)), num: string(g[:min(2, len(g))]), pos: m.Start(), end: m.End()}}
		}
	}
	if !own {
		if len(models) == 0 {
			return nil
		}
		if d.block != nil && d.block.IsMatch(t) {
			return nil
		}
	}
	src := "title"
	var first *numericModel
	var code string
	switch {
	case len(models) > 0:
		first = &models[0]
		code = first.code
		for _, x := range models {
			if x.num == first.num && runeLen(x.code) > runeLen(code) {
				code = x.code
			}
		}
	case own && pm != nil:
		code, src = pm.code, "mpn"
	default:
		return nil
	}
	gen := ""
	if first != nil && first.gen != "" {
		gen = first.gen
	} else if m := genRx.Search(t, 0); m != nil {
		switch {
		case m.Group(1) != "":
			gen = m.Group(1)
		case m.Group(2) != "":
			gen = m.Group(2)
		default:
			gen = m.Group(3)
		}
	} else if first != nil && vAfterRx.MatchAt(t[first.end:], 0) != nil {
		gen = "V"
	} else if pm != nil && pm.gen != "" && pm.code == code {
		gen = pm.gen
	}
	if d.noGenModels[code] && gen != "V" {
		gen = ""
	}
	configs := []string{}
	for _, c := range d.configs {
		if c.rx.IsMatch(t) || (c.name == "MOS" && ((first != nil && first.mos) || (pm != nil && strings.Contains(pyre.Lower(pm.mpn), "mos")))) {
			configs = append(configs, c.name)
		}
	}
	features := []string{}
	threaded := false
	for _, f := range d.features {
		if f.rx.IsMatch(t) {
			features = append(features, f.name)
			threaded = threaded || f.name == "Threaded Barrel"
		}
	}
	if pm != nil && !threaded && mpnTBRx.MatchString(pm.mpn) {
		features = append(features, "Threaded Barrel")
	}
	return &modelResult{code: code, family: d.models[code], gen: gen, configs: configs, features: features, src: src}
}

// ---------- terms mode (all other brands): ordered list of model patterns, first match wins ----------

func (d *modelDict) parseTerms(t []rune, brand string) *modelResult {
	if brand != "" && d.denyNames[modelKey(brand)] {
		return nil
	}
	own := (brand != "" && d.brandNames[modelKey(brand)]) || d.brandStart.IsMatch(t)
	if !own {
		if !d.brandAny.IsMatch(t) {
			return nil
		}
		if d.block != nil && d.block.IsMatch(t) {
			return nil
		}
	}
	var hit *termModel
	for i := range d.termModels {
		m := &d.termModels[i]
		if m.match.IsMatch(t) && !(m.not != nil && m.not.IsMatch(t)) {
			hit = m
			break
		}
	}
	if hit == nil {
		return nil
	}
	if !own && d.otherBrandModels != nil && !d.otherBrandModels[hit.name] {
		return nil
	}
	gen := ""
	for _, g := range d.gens {
		if g.rx.IsMatch(t) {
			gen = g.name
			break
		}
	}
	configs := []string{}
	for _, c := range d.configs {
		if !hit.configs[c.name] {
			continue
		}
		implied := false
		if gen != "" {
			for _, g := range d.configImplied[c.name][hit.family] {
				implied = implied || g == gen
			}
		}
		if c.rx.IsMatch(t) || implied || hit.impliedConfigs[c.name] {
			configs = append(configs, c.name)
		}
	}
	features := []string{}
	for _, f := range d.features {
		if f.rx.IsMatch(t) {
			features = append(features, f.name)
		}
	}
	return &modelResult{code: hit.name, family: hit.family, gen: gen, configs: configs, features: features, src: "title"}
}

var leafWordRx = mustRx(`[a-z0-9.&+]+`, false)

func wordSet(s string) map[string]bool {
	return stringSet(leafWordRx.FindAllStrings([]rune(pyre.Lower(s))))
}

func (d *modelDict) leaf(r *modelResult) string {
	var s string
	if d.terms {
		s = r.code
		if r.gen != "" {
			s += " " + r.gen
		}
	} else {
		s = d.brand + " " + r.code
		if r.gen == "V" {
			s += " V"
		} else if r.gen != "" {
			s += " Gen " + r.gen
		}
	}
	for _, c := range r.configs {
		// skip a config already spelled out in the name ("Plus Shield" on "S&W M&P Shield Plus")
		if d.leafRx[c].MatchString(s) {
			continue
		}
		have := wordSet(s)
		subset := true
		for w := range wordSet(c) {
			subset = subset && have[w]
		}
		if !subset {
			s += " " + c
		}
	}
	needed := stringSet(d.needed[r.family])
	features := stringSet(r.features)
	for _, f := range d.featureOrder {
		if features[f] && needed[f] {
			s += " " + f
		}
	}
	return s
}

// IdentifyModel finds the brand, model family and model in a firearm product title. It uses Brand (the
// manufacturer, which helps with titles that do not name it), MPN (Glock part numbers encode the model and the
// generation) and SKU from opts, which may be nil.
//
// It returns nil when no covered brand and model is recognized. Only firearms are covered: titles of accessories,
// parts, airguns and airsoft guns return nil.
func IdentifyModel(title string, opts *Options) *ModelMatch {
	var o Options
	if opts != nil {
		o = *opts
	}
	return identifyModel(clean(title), clean(o.Brand), clean(o.MPN), clean(o.SKU))
}

func identifyModel(title, brand, mpn, sku string) *ModelMatch {
	if title == "" || airgunRx.MatchString(title) || isAccessory(title) {
		return nil
	}
	base := normBase(title)
	brandKey := ""
	if brand != "" {
		brandKey = modelKey(brand)
	}
	type hit struct {
		d    *modelDict
		r    *modelResult
		rank [2]int
	}
	var best *hit
	for _, d := range dicts() {
		t := []rune(d.norm(base))
		if d.exclude != nil && d.exclude.IsMatch(t) {
			continue
		}
		var r *modelResult
		if d.terms {
			r = d.parseTerms(t, brand)
		} else if (brand != "" && d.brandNames[brandKey]) || d.brandAny.IsMatch(t) {
			// the numeric dictionary (Glock) only runs when the brand is given or named in the title;
			// otherwise part-number decoding picks up other brands' codes (CVA "PR3110S" -> "Glock 31")
			r = d.parseNumeric(t, title, brand, mpn, sku)
		}
		if r == nil {
			continue
		}
		// which matching dictionary wins: the product's own brand -> brand at the start of the title -> brand
		// mentioned earlier
		var rank [2]int
		switch m := d.brandAny.Search(t, 0); {
		case brand != "" && d.brandNames[brandKey]:
			rank = [2]int{0, 0}
		case m != nil && m.Start() <= 2:
			rank = [2]int{1, 0}
		case m != nil:
			rank = [2]int{2, m.Start()}
		default:
			rank = [2]int{2, 1000000}
		}
		if best == nil || rank[0] < best.rank[0] || (rank[0] == best.rank[0] && rank[1] < best.rank[1]) {
			best = &hit{d, r, rank}
		}
	}
	if best == nil {
		return nil
	}
	d, r := best.d, best.r
	fam := d.familyDisplay[r.family]
	if fam == "" {
		if d.terms {
			fam = r.family
		} else {
			fam = d.brand + " " + r.family
		}
	}
	return &ModelMatch{
		Brand: d.brand, Family: fam, Model: d.leaf(r), Generation: r.gen,
		Configs: append([]string{}, r.configs...), Features: append([]string{}, r.features...), Source: r.src,
	}
}
