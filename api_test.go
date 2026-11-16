package gunsnprices_test

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/gunsnprices/gunsnprices-go"
)

// The unit tests of the Python package (tests/test_api.py), plus Go-specific checks.

func TestIdentifyModel(t *testing.T) {
	cases := []struct{ title, brand, mpn, model string }{
		{"Glock 19 Gen 5 MOS 9mm 4.02in 15rd Pistol", "", "", "Glock 19 Gen 5 MOS"},
		{"G19 Gen5 9mm 15rd FS", "Glock", "", "Glock 19 Gen 5"},
		{"Glock 43X 9mm 3.41in 10rd Black", "", "", "Glock 43X"},
		{"GLOCK 9MM PISTOL 15RD", "Glock", "PA195S203", "Glock 19 Gen 5"},
		{"Sig Sauer P365 XL 9mm 3.7in 12rd Pistol", "", "", "Sig Sauer P365 XL"},
		{"Ruger GP100 .357 Mag 4.2in 6rd Revolver", "", "", "Ruger GP100"},
	}
	for _, c := range cases {
		m := gunsnprices.IdentifyModel(c.title, &gunsnprices.Options{Brand: c.brand, MPN: c.mpn})
		if m == nil || m.Model != c.model {
			t.Errorf("%q: got %+v, want %q", c.title, m, c.model)
		}
	}
}

func TestIdentifyModelRejects(t *testing.T) {
	for _, title := range []string{
		"Umarex Glock 19 CO2 BB Pistol .177",
		"Hornady 9mm 115gr FTX Critical Defense 25rd",
		"Glock 19 holster",
		"Sig P365 XL 12rd Magazine 9mm",
		"Magpul PMAG for Glock 19",
		"",
	} {
		if m := gunsnprices.IdentifyModel(title, nil); m != nil {
			t.Errorf("%q: got %+v, want nil", title, m)
		}
	}
}

func TestIncludedAccessoriesKeepTheGun(t *testing.T) {
	if m := gunsnprices.IdentifyModel("Glock 19 Gen 5 9mm 15rd w/ 3 magazines", nil); m == nil || m.Model != "Glock 19 Gen 5" {
		t.Errorf("got %+v", m)
	}
	if gunsnprices.IdentifyModel("Daniel Defense DDM4 V7 (No Magazine)", nil) == nil {
		t.Error("DDM4 V7 not found")
	}
}

func TestMPNSource(t *testing.T) {
	m := gunsnprices.IdentifyModel("GLOCK 9MM PISTOL 15RD", &gunsnprices.Options{Brand: "Glock", MPN: "PA195S203"})
	if m == nil || m.Source != "mpn" || m.Family != "Glock 19" {
		t.Errorf("got %+v", m)
	}
	m = gunsnprices.IdentifyModel("GLOCK 9MM PISTOL 15RD", &gunsnprices.Options{Brand: "Glock", MPN: "UA4350201"})
	if m == nil || m.Model != "Glock 43" || m.Source != "mpn" {
		t.Errorf("got %+v", m)
	}
}

func names(cs []gunsnprices.Caliber) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func TestFindCalibers(t *testing.T) {
	cases := []struct {
		text  string
		names []string
	}{
		{"Ruger GP100 .357 Mag / .38 Special 4.2in Revolver", []string{".357 Mag", ".38 S&W Special"}},
		{"Federal American Eagle 9mm Luger 115gr FMJ 50rd", []string{"9×19mm Parabellum"}},
		{"Hornady 6.5 Creedmoor 143gr ELD-X", []string{"6.5mm Creedmoor"}},
		{"PSA AR-15 300 Blackout 16in Rifle", []string{".300 AAC Blackout"}},
		{"Kent 12G Fasteel 3in BBB", []string{"12 Gauge"}},
		{`Beretta 686 O/U 12/28 BL/WD 3"`, []string{"12 Gauge"}},
		{"Winchester 308 150gr PowerMax Bonded 20rds", []string{".308 Win"}},
		{`Remington 7mm RM 26" Bolt Rifle`, []string{"7mm Remington Magnum"}},
		{"Henry Big Boy .44 Mag/.44 Spl 20in", []string{".44 Magnum", ".44 S&W Special"}},
		{"Model 1911 Range Officer Parkerized", []string{}},
		{"Umarex 12g CO2 Cartridges 15 pack", []string{}}, // a 12-gram CO2 cartridge is not 12 gauge
	}
	for _, c := range cases {
		got := names(gunsnprices.FindCalibers(c.text))
		if strings.Join(got, "|") != strings.Join(c.names, "|") {
			t.Errorf("%q: got %q, want %q", c.text, got, c.names)
		}
	}
	if c, ok := gunsnprices.FindCaliber("Ruger GP100 .357 Mag / .38 Special"); !ok || c.ID != "357mag" {
		t.Errorf("FindCaliber: %+v %v", c, ok)
	}
	if _, ok := gunsnprices.FindCaliber("no caliber here"); ok {
		t.Error("FindCaliber found something in a title without a caliber")
	}
}

func TestNormalizeCaliber(t *testing.T) {
	cases := map[string]string{
		"9mm Luger":              "9x19mmparabellum",
		"5.56 NATO":              "5-56x45mmnato",
		"12 GA":                  "12gauge",
		"308 WIN (7.62X51 NATO)": "308win",
	}
	for in, id := range cases {
		if c, ok := gunsnprices.NormalizeCaliber(in); !ok || c.ID != id {
			t.Errorf("%q: got %+v %v, want %s", in, c, ok, id)
		}
	}
	c, _ := gunsnprices.NormalizeCaliber("308 WIN (7.62X51 NATO)")
	want := gunsnprices.Caliber{ID: "308win", Name: ".308 Win", Type: "rifle", URL: "https://gunsnprices.com/ammo/rifle-ammo/308-win"}
	if c != want {
		t.Errorf("got %+v", c)
	}
	if c, ok := gunsnprices.NormalizeCaliber("nothing here"); ok {
		t.Errorf("got %+v", c)
	}
}

func TestLists(t *testing.T) {
	brands := gunsnprices.ListBrands()
	if len(brands) != 50 || !contains(brands, "Glock") {
		t.Errorf("ListBrands: %d %v", len(brands), brands)
	}
	cals := gunsnprices.ListCalibers()
	seen := map[string]bool{}
	for i, c := range cals {
		seen[c.ID] = true
		if i > 0 && cals[i-1].Name > c.Name {
			t.Errorf("not sorted: %q before %q", cals[i-1].Name, c.Name)
		}
	}
	if len(cals) != 293 || len(seen) != len(cals) {
		t.Errorf("ListCalibers: %d calibers, %d ids", len(cals), len(seen))
	}
	if c, ok := gunsnprices.CaliberByID("9x19mmparabellum"); !ok || c.Name != "9×19mm Parabellum" {
		t.Errorf("CaliberByID: %+v %v", c, ok)
	}
	// the returned slices are copies
	cals[0].Name = "changed"
	if gunsnprices.ListCalibers()[0].Name == "changed" {
		t.Error("ListCalibers returned the internal slice")
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestMagazineCapacity(t *testing.T) {
	cases := map[string]int{
		"Glock 19 Gen5 9mm 4.02in 15+1":                       15,
		"Kimber CDS9 9mm 3.18in 13rd/15rd Black":              15,
		"Savage 110 Scout 450 Bushmaster 4 Round Bolt Action": 4,
		"Springfield Hellcat Pro OSP 9mm 3.7in 15rd/17rd":     17,
		"Ruger 10/22 Carbine 22LR":                            0,
	}
	for title, want := range cases {
		got, ok := gunsnprices.MagazineCapacity(title)
		if ok != (want != 0) || got != want {
			t.Errorf("%q: got %d %v, want %d", title, got, ok, want)
		}
	}
}

func TestBarrelLength(t *testing.T) {
	cases := map[string]float64{
		"Ruger GP100 .357 Mag 4.2in Revolver":             4.2,
		`Beretta A300 Ultima 12 GA 3" Chamber 28" Barrel`: 28.0,
		`Mossberg 590 Persuader 12/20 BL/SY 3"`:           20.0,
		`Colt Python 4 1/4" .357`:                         4.25,
		"Ruger 10/22 SBR 22LR 8in 15rd":                   8.0,
		`Bergara B-14 24" 1:8" twist`:                     24.0,
		"Glock 19 Gen5 9mm":                               0,
	}
	for title, want := range cases {
		got, ok := gunsnprices.BarrelLength(title)
		if ok != (want != 0) || got != want {
			t.Errorf("%q: got %v %v, want %v", title, got, ok, want)
		}
	}
}

func TestRoundsPerBox(t *testing.T) {
	cases := map[string]int{
		"Federal American Eagle 9mm 115gr FMJ, 50/Box":   50,
		"Prvi Partizan 5.56x45mm M193 55gr 1,000 Rounds": 1000,
		"Lapua 8x57 IRS 180gr Naturalis Box of 20":       20,
		"Barnaul 7.62x39 123gr FMJ 20/25":                20,
		"Winchester Super-X .30-30 Win 150gr":            0,
	}
	for title, want := range cases {
		got, ok := gunsnprices.RoundsPerBox(title)
		if ok != (want != 0) || got != want {
			t.Errorf("%q: got %d %v, want %d", title, got, ok, want)
		}
	}
}

func TestCostPerRound(t *testing.T) {
	if v, ok := gunsnprices.CostPerRound(18.99, 50); !ok || v != 0.3798 {
		t.Errorf("got %v %v", v, ok)
	}
	if v, ok := gunsnprices.CostPerRound(449.99, 1000); !ok || v != 0.45 {
		t.Errorf("got %v %v", v, ok)
	}
	// Python's round() rounds the exact binary value: 0.09/200 is stored as 0.000449999..., so it gives 0.0004
	// where rounding x*10000 would give 0.0005
	if v, ok := gunsnprices.CostPerRound(0.09, 200); !ok || v != 0.0004 {
		t.Errorf("got %v %v", v, ok)
	}
	for _, c := range []struct {
		price  float64
		rounds int
	}{{0, 50}, {10, 0}, {-5, 10}, {10, -1}} {
		if _, ok := gunsnprices.CostPerRound(c.price, c.rounds); ok {
			t.Errorf("%v: expected no value", c)
		}
	}
	if v, ok := gunsnprices.CostPerRound(math.NaN(), 10); !ok || !math.IsNaN(v) {
		t.Errorf("NaN price: %v %v (Python returns nan)", v, ok)
	}
}

func TestLooksLikeAmmo(t *testing.T) {
	if !gunsnprices.LooksLikeAmmo("Federal 9mm 115gr FMJ 50/Box") {
		t.Error("ammo title not recognized")
	}
	if gunsnprices.LooksLikeAmmo("Glock 19 Gen 5 9mm 15rd Pistol") {
		t.Error("pistol title taken for ammo")
	}
}

func TestParseTitleFirearm(t *testing.T) {
	info := gunsnprices.ParseTitle("Glock 19 Gen5 MOS 9mm 4.02in 15+1 Black", nil)
	if info.Kind != gunsnprices.Firearm || info.Brand != "Glock" || info.Model == nil || info.Model.Model != "Glock 19 Gen 5 MOS" {
		t.Fatalf("got %+v", info)
	}
	if info.Caliber == nil || info.Caliber.Name != "9×19mm Parabellum" {
		t.Errorf("caliber %+v", info.Caliber)
	}
	if info.Capacity == nil || *info.Capacity != 15 || info.BarrelLength == nil || *info.BarrelLength != 4.02 || info.Rounds != nil {
		t.Errorf("numbers %+v", info)
	}
}

func TestParseTitleAmmo(t *testing.T) {
	info := gunsnprices.ParseTitle("Federal American Eagle 9mm Luger 115gr FMJ, 50/Box", &gunsnprices.Options{Price: 18.99})
	if info.Kind != gunsnprices.Ammo || info.Model != nil {
		t.Fatalf("got %+v", info)
	}
	if info.Rounds == nil || *info.Rounds != 50 || info.CostPerRound == nil || *info.CostPerRound != 0.3798 {
		t.Errorf("numbers %+v", info)
	}
	if info.Caliber == nil || info.Caliber.ID != "9x19mmparabellum" {
		t.Errorf("caliber %+v", info.Caliber)
	}
}

func TestParseTitleKindOption(t *testing.T) {
	info := gunsnprices.ParseTitle("Glock 19 9mm 15rd", &gunsnprices.Options{Kind: gunsnprices.Ammo})
	if info.Kind != gunsnprices.Ammo || info.Model != nil || info.Rounds == nil || *info.Rounds != 15 {
		t.Errorf("got %+v", info)
	}
	// an unknown kind is ignored, like in Python
	info = gunsnprices.ParseTitle("Glock 19 9mm 15rd", &gunsnprices.Options{Kind: "pistol"})
	if info.Kind != gunsnprices.Firearm {
		t.Errorf("got %+v", info)
	}
}

func TestJSONLikePython(t *testing.T) {
	info := gunsnprices.ParseTitle(`Barrett MRAD .300 PRC 26" Barrel 10-Rounds`, nil)
	b, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, part := range []string{`"title":`, `"kind":"firearm"`, `"barrel_length":26.0`, `"capacity":10`,
		`"rounds":null`, `"cost_per_round":null`, `"configs":[]`} {
		if !strings.Contains(s, part) {
			t.Errorf("%s: missing %s", s, part)
		}
	}
	if !strings.HasPrefix(s, `{"title":`) || strings.Index(s, `"calibers"`) > strings.Index(s, `"capacity"`) {
		t.Errorf("field order: %s", s)
	}
	// and back
	var back gunsnprices.ProductInfo
	if err := json.Unmarshal(b, &back); err != nil || *back.BarrelLength != 26 || back.Model.Model != info.Model.Model {
		t.Errorf("round trip: %v %+v", err, back)
	}
	empty, _ := json.Marshal(gunsnprices.ProductInfo{})
	if !strings.Contains(string(empty), `"calibers":[]`) {
		t.Errorf("zero value: %s", empty)
	}
}

func TestInvalidUTF8(t *testing.T) {
	info := gunsnprices.ParseTitle("Glock 19 \xff\xfe Gen 5 9mm", nil)
	if info.Model == nil || info.Model.Model != "Glock 19 Gen 5" {
		t.Errorf("got %+v", info.Model)
	}
	if !strings.Contains(info.Title, "�") {
		t.Errorf("title %q", info.Title)
	}
}

func TestLongInput(t *testing.T) {
	title := strings.Repeat("Glock 19 Gen 5 9mm 15rd 4.02in ", 2000)
	info := gunsnprices.ParseTitle(title, nil)
	if info.Model == nil || info.Capacity == nil {
		t.Errorf("got %+v", info.Model)
	}
}

func TestConcurrentUse(t *testing.T) {
	titles := []string{
		"Glock 19 Gen 5 MOS 9mm 4.02in 15rd Pistol PA195S203MOS",
		"Federal American Eagle 5.56x45mm 55gr FMJ 1000 Rounds",
		"Sig Sauer P365 XL 9mm 3.7\" 12+1 Optic Ready",
		`Beretta 686 O/U 12/28 BL/WD 3"`,
	}
	want := make([]string, len(titles))
	for i, title := range titles {
		b, _ := json.Marshal(gunsnprices.ParseTitle(title, nil))
		want[i] = string(b)
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 200; k++ {
				i := k % len(titles)
				b, _ := json.Marshal(gunsnprices.ParseTitle(titles[i], nil))
				if string(b) != want[i] {
					t.Errorf("concurrent result differs for %q", titles[i])
					return
				}
			}
		}()
	}
	wg.Wait()
}
