# gunsnprices for Go

[![CI](https://github.com/gunsnprices/gunsnprices-go/actions/workflows/ci.yml/badge.svg)](https://github.com/gunsnprices/gunsnprices-go/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-MIT%20%2B%20CC%20BY%204.0%20data-blue)](#license)

Parse firearm and ammunition product titles into structured data: brand, model, caliber, magazine capacity, barrel
length, rounds per box and cost per round.

```go
import "github.com/gunsnprices/gunsnprices-go"

info := gunsnprices.ParseTitle("Glock 19 Gen 5 MOS 9mm 4.02in 15rd Pistol PA195S203MOS", nil)
info.Model.Model    // "Glock 19 Gen 5 MOS"
info.Caliber.Name   // "9×19mm Parabellum"
*info.Capacity      // 15
*info.BarrelLength  // 4.02
```

This is the title parser behind [GunsNPrices](https://gunsnprices.com), a price comparison site that tracks 40+ US gun
and ammo stores. Store titles are messy (`G19 GEN5 9MM 15RD FS`, `O/U 12/28 BL/WD 3"`, `Federal 308 150gr ... 20rds`),
and matching the same product across stores starts with reading them reliably. The model dictionaries and the caliber
list in this module are the ones the site uses to group listings, so you get the same model names you see on
[gunsnprices.com](https://gunsnprices.com).

It is a port of the [Python package](https://pypi.org/project/gunsnprices/) and gives identical results: the tests
check the same 1,150 recorded store titles as the other versions, and it agrees with the Python package on every
public function for all 63,755 titles of the GunsNPrices catalog. Standard library only, data embedded in the binary,
safe for concurrent use.

Documentation: [pkg.go.dev/github.com/gunsnprices/gunsnprices-go](https://pkg.go.dev/github.com/gunsnprices/gunsnprices-go).
Other languages, plugins and the open datasets: [gunsnprices.com/developers](https://gunsnprices.com/developers).

## Install

```bash
go get github.com/gunsnprices/gunsnprices-go
```

Requires Go 1.26 or later. The command line tool:

```bash
go install github.com/gunsnprices/gunsnprices-go/cmd/gunsnprices@latest
```

## What it recognizes

| Field | Example in a title | Result |
|---|---|---|
| Firearm or ammo | `... 150gr ... 20rds` | `Kind: Ammo` |
| Brand and model (50 brands) | `G19 Gen5 MOS`, part number `PA195S203` | `Glock 19 Gen 5 MOS` |
| Calibers (293) | `.357 Mag / .38 Spl`, `6.5 CM`, `12ga`, `300 BLK`, `7MM RM` | `.357 Mag`, `.38 S&W Special`, ... |
| Magazine capacity | `15+1`, `17RD`, `13rd/15rd`, `10-shot` | `15`, `17`, `15`, `10` |
| Barrel length, inches | `4.02in`, `16 1/2"`, `22 BBL`, `O/U 12/28` | `4.02`, `16.5`, `22`, `28` |
| Rounds per box | `50/Box`, `Box of 20`, `1,000 Rounds`, `20/25` | `50`, `20`, `1000`, `20` |
| Cost per round | price `449.99` + `1000 Rounds` | `0.45` |

Brands with model dictionaries: Barrett, Benelli, Beretta, Bergara, Browning, Cadex, Canik, Charles Daly, Chiappa,
Christensen Arms, Cimarron, CMMG, Colt, CVA, CZ, Daniel Defense, Desert Eagle, Diamondback, Fierce, FN, Glock,
Great Lakes, Henry, Heritage, HK, Howa, Kel-Tec, Kimber, Kriss, Marlin, Mossberg, Remington, Retay, Rossi, Ruger, Savage,
SCCY, Seekins, Shadow Systems, Sig Sauer, Smith & Wesson, Springfield, Stevens, Stoeger, Taurus, Tikka, Tristar, Walther,
Weatherby, Winchester.

## Usage

### Everything at once

```go
info := gunsnprices.ParseTitle("Federal American Eagle 5.56x45mm 55gr FMJ 1000 Rounds",
	&gunsnprices.Options{Price: 449.99})
info.Kind           // gunsnprices.Ammo
info.Caliber.Name   // "5.56×45mm NATO"
*info.Rounds        // 1000
*info.CostPerRound  // 0.45

b, _ := json.Marshal(info) // same fields and number format as the Python package's to_dict()
```

`ParseTitle(title string, opts *Options) ProductInfo`; `opts` may be `nil`. `Options` has `Brand` (the manufacturer
field of a feed: it helps with titles that don't name the brand), `MPN`, `SKU`, `Price` (gives the cost per round for
ammo) and `Kind` (`gunsnprices.Firearm` or `gunsnprices.Ammo` to skip the guess). `ProductInfo` has:

| Field | Type | Notes |
|---|---|---|
| `Kind` | `Kind` | `Firearm` or `Ammo` |
| `Brand` | `string` | brand of the identified model, else `Options.Brand`, else `""` |
| `Model` | `*ModelMatch` | firearms only |
| `Caliber` | `*Caliber` | first caliber in the title |
| `Calibers` | `[]Caliber` | all calibers, in order |
| `Capacity` | `*int` | magazine capacity (for `15+1` it is 15), firearms only |
| `BarrelLength` | `*float64` | inches, firearms only |
| `Rounds` | `*int` | rounds per box, ammo only |
| `CostPerRound` | `*float64` | ammo with `Options.Price` only |

A field that does not apply or was not found is `nil`.

### Models

```go
m := gunsnprices.IdentifyModel(`Sig Sauer P365 XL 9mm 3.7" 12+1 Optic Ready`, nil)
m.Brand, m.Family, m.Model  // "Sig Sauer", "Sig Sauer P365", "Sig Sauer P365 XL"

// Glock part numbers encode the model and the generation
m = gunsnprices.IdentifyModel("GLOCK 9MM PISTOL 15RD", &gunsnprices.Options{Brand: "Glock", MPN: "UA4350201"})
m.Model, m.Source  // "Glock 43", "mpn"

gunsnprices.IdentifyModel("Glock 19 holster", nil)               // nil
gunsnprices.IdentifyModel("Umarex Glock 19 CO2 BB pistol", nil)  // nil
```

A `ModelMatch` has `Brand`, `Family`, `Model` (the most specific name), `Generation`, `Configs` (such as `MOS`,
`Compact`), `Features` (such as `Threaded Barrel`, `FDE`) and `Source` (`"title"` or `"mpn"`). Accessories, parts,
airguns and airsoft are rejected. `ListBrands()` returns the 50 brands.

### Calibers

```go
for _, c := range gunsnprices.FindCalibers("Henry Big Boy .44 Mag/.44 Spl 20in") {
	fmt.Println(c.Name) // ".44 Magnum", ".44 S&W Special"
}

c, ok := gunsnprices.NormalizeCaliber("308 WIN (7.62X51 NATO)")
// gunsnprices.Caliber{ID: "308win", Name: ".308 Win", Type: "rifle",
//                     URL: "https://gunsnprices.com/ammo/rifle-ammo/308-win"}, true
```

Each `Caliber` has a stable `ID`, a display `Name`, a `Type` (`handgun`, `rifle`, `rimfire`, `shotgun`, `airgun`,
`muzzleloader`) and the `URL` of its GunsNPrices price comparison page. `NormalizeCaliber` is meant for caliber fields
in product feeds; `FindCalibers` and `FindCaliber` search free text. `ListCalibers()` returns all 293 and
`CaliberByID("9x19mmparabellum")` looks one up.

### Numbers

```go
gunsnprices.MagazineCapacity("Springfield Hellcat Pro OSP 9mm 3.7in 15rd/17rd")  // 17, true
gunsnprices.BarrelLength(`Mossberg 590 Persuader 12/20 BL/SY 3"`)               // 20, true (3" is the chamber)
gunsnprices.RoundsPerBox("Winchester 308 150gr PowerMax Bonded 20rds")          // 20, true
gunsnprices.CostPerRound(31.99, 20)                                             // 1.5995, true
gunsnprices.LooksLikeAmmo("Federal 9mm 115gr FMJ 50/Box")                       // true
```

The second result is `false` when the title has no such number.

### Command line

```bash
gunsnprices "Ruger GP100 .357 Mag / .38 Special 4.2in 6-shot Revolver"
gunsnprices -price 18.99 "Federal 9mm 115gr FMJ 50/Box"
gunsnprices < titles.txt > parsed.jsonl      # one title per line in, JSON Lines out
```

Flags: `-brand`, `-mpn`, `-sku`, `-price`, `-kind firearm|ammo`, `-version`. The indented output for a title argument
has the same fields, order and number format as the Python command's.

## Performance

The data is read and the rules are compiled on the first call (about 70 ms); call `ParseTitle` once at startup if that
matters. After that `ParseTitle` takes about 0.1 ms per title on one core (Go 1.26, Apple M2, `go test -bench .` over
the 1,150 recorded titles). All functions are safe for concurrent use.

## How it matches Python

The rules are regular expressions written for Python's `re` module. Go's `regexp` package (RE2) has no look-behind or
look-ahead, which the rules use, so the module includes a small backtracking engine (`internal/pyre`) that matches the
way Python's does: leftmost-first alternation, greedy and lazy repetition, fixed-width look-behind, `$` before a final
newline, and Python's `\d`, `\w`, `\s`, `\b` and `re.IGNORECASE` (`i` also matches `ı` and `İ`, `ss` does not match
`ß`). `str.lower()`, `html.unescape()` and `round()` behave like Python's too. The Unicode tables come from Python 3.13
(Unicode 15.1), the same as the other ports, so the results don't depend on your Go version.

The engine is tested against Python's `re` on 27,124 pattern and text pairs (`internal/pyre/testdata`), and the whole
module against the Python package on 108,004 inputs: the recorded titles with Unicode edits, all catalog titles, and the
caliber and model names of the open datasets (see [Development](#development)).

## Accuracy

Measured with the reference implementation on 50,202 firearm and 13,555 ammunition listings from 40+ stores on
gunsnprices.com (October 2026), against the attributes stored on the site:

| What | Found in the title | Right when found |
|---|---|---|
| Model (32,317 listings with a known model) | 99.97% | 99.8% same name as the site |
| Caliber, firearms | 81% | 97.8% |
| Caliber, ammo | 91% | 94.9% |
| Magazine capacity | 46% | 95.6% |
| Barrel length | 58% | 94.4% |
| Rounds per box | 80% | 96.6% |

"Found" is limited by the titles themselves: many stores don't put the caliber or barrel length in the title at all.
Most of the "wrong" cases we checked by hand are errors in the store data, not in the parser.

## Data

The model dictionaries and caliber list are embedded in the module (`data/*.json`, the same files as the Python
package). The same data, with listing counts and price levels, is published as open datasets:

- Hugging Face: [gunsnprices/firearm-calibers](https://huggingface.co/datasets/gunsnprices/firearm-calibers),
  [gunsnprices/firearm-models](https://huggingface.co/datasets/gunsnprices/firearm-models)
- GitHub: [gunsnprices/firearm-datasets](https://github.com/gunsnprices/firearm-datasets)

## Limitations

- Titles from US retailers, in English.
- Models are covered for the 50 brands above. Other brands still get calibers and numbers.
- Built for firearms and ammunition. Magazines, parts and accessories get calibers and numbers but no model.
- Strings are read as UTF-8; invalid bytes are replaced with U+FFFD before parsing.

## Development

```bash
go test ./...                     # 1,150 recorded titles, unit tests, examples, engine tests
go test -bench . -run '^$'        # speed

# compare every public function with the Python package (add --catalog DIR with lib_guns.tsv / lib_ammo.tsv
# and --datasets DIR for more inputs)
pip install gunsnprices==0.1.2
python3.13 tools/make_corpus.py /tmp/corpus.jsonl
GUNSNPRICES_CORPUS=/tmp/corpus.jsonl go test -run Crosscheck -v

# regenerate the Unicode and HTML tables and the engine test cases from Python 3.13
python3.13 tools/gen_tables.py > internal/pyre/tables.go
python3.13 tools/gen_pyre_cases.py | gzip -n -9 > internal/pyre/testdata/cases.jsonl.gz
```

## License

The code is under the [MIT License](LICENSE). The model and caliber data in `data/` is under
[CC BY 4.0](LICENSE-DATA): you can use it for anything, including commercial use, if you credit
**GunsNPrices (gunsnprices.com)** with a link.

Questions, corrections and missing models: open an issue or write to app@gunsnprices.com.
Current gun and ammo prices: [gunsnprices.com](https://gunsnprices.com).
