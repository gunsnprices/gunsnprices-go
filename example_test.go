package gunsnprices_test

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gunsnprices/gunsnprices-go"
)

func ExampleParseTitle() {
	info := gunsnprices.ParseTitle("Glock 19 Gen 5 MOS 9mm 4.02in 15rd Pistol PA195S203MOS", nil)
	fmt.Println(info.Kind)
	fmt.Println(info.Model.Model)
	fmt.Println(info.Caliber.Name)
	fmt.Println(*info.Capacity, *info.BarrelLength)
	// Output:
	// firearm
	// Glock 19 Gen 5 MOS
	// 9×19mm Parabellum
	// 15 4.02
}

func ExampleParseTitle_ammo() {
	info := gunsnprices.ParseTitle("Federal American Eagle 5.56x45mm 55gr FMJ 1000 Rounds",
		&gunsnprices.Options{Price: 449.99})
	fmt.Println(info.Kind)
	fmt.Println(info.Caliber.Name)
	fmt.Println(*info.Rounds, *info.CostPerRound)
	// Output:
	// ammo
	// 5.56×45mm NATO
	// 1000 0.45
}

func ExampleParseTitle_options() {
	// a store title that does not name the brand: the manufacturer field of the feed helps
	info := gunsnprices.ParseTitle("G19 GEN5 9MM 15RD FS", &gunsnprices.Options{Brand: "Glock"})
	fmt.Println(info.Brand, "|", info.Model.Model)
	// Output: Glock | Glock 19 Gen 5
}

func ExampleProductInfo_MarshalJSON() {
	info := gunsnprices.ParseTitle(`Mossberg 590 Persuader 12/20 BL/SY 3"`, nil)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(info); err != nil {
		panic(err)
	}
	// Output:
	// {
	//   "title": "Mossberg 590 Persuader 12/20 BL/SY 3\"",
	//   "kind": "firearm",
	//   "brand": "Mossberg",
	//   "model": {
	//     "brand": "Mossberg",
	//     "family": "Mossberg 590",
	//     "model": "Mossberg 590 Persuader",
	//     "generation": "",
	//     "configs": [
	//       "Persuader"
	//     ],
	//     "features": [],
	//     "source": "title"
	//   },
	//   "caliber": {
	//     "id": "12gauge",
	//     "name": "12 Gauge",
	//     "type": "shotgun",
	//     "url": "https://gunsnprices.com/ammo/shotgun-ammo/12-gauge"
	//   },
	//   "calibers": [
	//     {
	//       "id": "12gauge",
	//       "name": "12 Gauge",
	//       "type": "shotgun",
	//       "url": "https://gunsnprices.com/ammo/shotgun-ammo/12-gauge"
	//     }
	//   ],
	//   "capacity": null,
	//   "barrel_length": 20.0,
	//   "rounds": null,
	//   "cost_per_round": null
	// }
}

func ExampleIdentifyModel() {
	m := gunsnprices.IdentifyModel(`Sig Sauer P365 XL 9mm 3.7" 12+1 Optic Ready`, nil)
	fmt.Println(m.Brand, "|", m.Family, "|", m.Model)

	// accessories, parts and airguns are not firearms
	fmt.Println(gunsnprices.IdentifyModel("Glock 19 holster", nil))
	fmt.Println(gunsnprices.IdentifyModel("Umarex Glock 19 CO2 BB pistol", nil))
	// Output:
	// Sig Sauer | Sig Sauer P365 | Sig Sauer P365 XL
	// <nil>
	// <nil>
}

func ExampleIdentifyModel_partNumber() {
	// Glock part numbers encode the model and the generation
	m := gunsnprices.IdentifyModel("GLOCK 9MM PISTOL 15RD", &gunsnprices.Options{Brand: "Glock", MPN: "UA4350201"})
	fmt.Println(m.Model, m.Source)
	// Output: Glock 43 mpn
}

func ExampleFindCalibers() {
	for _, c := range gunsnprices.FindCalibers("Henry Big Boy .44 Mag/.44 Spl 20in") {
		fmt.Println(c.ID, "|", c.Name, "|", c.Type)
	}
	// Output:
	// 44magnum | .44 Magnum | handgun
	// 44swspecial | .44 S&W Special | handgun
}

func ExampleFindCaliber() {
	c, ok := gunsnprices.FindCaliber(`Beretta 686 O/U 12/28 BL/WD 3"`)
	fmt.Println(c.Name, ok)
	// Output: 12 Gauge true
}

func ExampleNormalizeCaliber() {
	c, ok := gunsnprices.NormalizeCaliber("308 WIN (7.62X51 NATO)")
	fmt.Println(ok, c.Name, c.URL)
	_, ok = gunsnprices.NormalizeCaliber("Black")
	fmt.Println(ok)
	// Output:
	// true .308 Win https://gunsnprices.com/ammo/rifle-ammo/308-win
	// false
}

func ExampleListCalibers() {
	cals := gunsnprices.ListCalibers()
	fmt.Println(len(cals), cals[0].Name)
	// Output: 293 .17 HM2
}

func ExampleListBrands() {
	brands := gunsnprices.ListBrands()
	fmt.Println(len(brands), brands[0], brands[len(brands)-1])
	// Output: 50 Barrett Winchester
}

func ExampleMagazineCapacity() {
	n, ok := gunsnprices.MagazineCapacity("Springfield Hellcat Pro OSP 9mm 3.7in 15rd/17rd")
	fmt.Println(n, ok)
	// Output: 17 true
}

func ExampleBarrelLength() {
	// 3" is the chamber, 20" the barrel
	v, ok := gunsnprices.BarrelLength(`Mossberg 590 Persuader 12/20 BL/SY 3"`)
	fmt.Println(v, ok)
	v, _ = gunsnprices.BarrelLength(`Colt Python 4 1/4" .357`)
	fmt.Println(v)
	// Output:
	// 20 true
	// 4.25
}

func ExampleRoundsPerBox() {
	for _, title := range []string{
		"Winchester 308 150gr PowerMax Bonded 20rds",
		"Prvi Partizan 5.56x45mm M193 55gr 1,000 Rounds",
		"Barnaul 7.62x39 123gr FMJ 20/25",
	} {
		n, _ := gunsnprices.RoundsPerBox(title)
		fmt.Println(n)
	}
	// Output:
	// 20
	// 1000
	// 20
}

func ExampleCostPerRound() {
	v, ok := gunsnprices.CostPerRound(31.99, 20)
	fmt.Println(v, ok)
	// Output: 1.5995 true
}

func ExampleLooksLikeAmmo() {
	fmt.Println(gunsnprices.LooksLikeAmmo("Federal 9mm 115gr FMJ 50/Box"))
	fmt.Println(gunsnprices.LooksLikeAmmo("Glock 19 Gen 5 9mm 15rd Pistol"))
	// Output:
	// true
	// false
}
