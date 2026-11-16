// Package gunsnprices parses firearm and ammunition product titles into structured data: brand, model, caliber,
// magazine capacity, barrel length, rounds per box and cost per round.
//
//	info := gunsnprices.ParseTitle("Glock 19 Gen 5 MOS 9mm 4.02in 15rd Pistol PA195S203MOS", nil)
//	info.Model.Model     // "Glock 19 Gen 5 MOS"
//	info.Caliber.Name    // "9×19mm Parabellum"
//	*info.Capacity       // 15
//	*info.BarrelLength   // 4.02
//
// It is the title parser behind GunsNPrices (https://gunsnprices.com), a price comparison site for US gun and ammo
// stores, ported from the Python package (https://pypi.org/project/gunsnprices/). The model dictionaries (50 brands)
// and the caliber list (293 calibers) are the ones the site uses, embedded in the module.
//
// # What it reads
//
//   - firearm or ammunition: [ParseTitle] guesses it ([LooksLikeAmmo]) unless [Options].Kind says;
//   - brand and model: [IdentifyModel], for the brands returned by [ListBrands]; it also decodes Glock part numbers
//     ([Options].MPN) and rejects accessories, parts, airguns and airsoft guns;
//   - calibers: [FindCalibers], [FindCaliber], and [NormalizeCaliber] for the caliber field of a product feed;
//   - numbers: [MagazineCapacity] ("15+1", "17RD", "13rd/15rd"), [BarrelLength] ('16 1/2"', "22 BBL", "O/U 12/28"),
//     [RoundsPerBox] ("50/Box", "Box of 20", "1,000 Rounds", "20/25") and [CostPerRound].
//
// # Compatibility with the Python package
//
// The rules are regular expressions written for Python's re module, which Go's regexp package cannot run (they use
// look-behind and look-ahead). The module includes a small backtracking engine with Python's matching semantics,
// and reproduces the Python string functions the rules depend on (str.lower, html.unescape, round). Its Unicode
// tables come from Python 3.13 (Unicode 15.1), the same as the other ports, so results do not depend on the Go
// version. The test suite checks 1,150 recorded store titles against the Python results, and the module agrees with
// the Python package on every public function for all titles of the GunsNPrices catalog.
//
// All functions are safe for concurrent use. The data is read and the rules are compiled on first use.
//
// # Data license
//
// The code is under the MIT License. The model and caliber data in the data directory is under CC BY 4.0: credit
// "GunsNPrices (gunsnprices.com)" with a link. See https://gunsnprices.com/developers for the other languages and
// the open datasets.
package gunsnprices
