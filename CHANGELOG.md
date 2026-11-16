# Changelog

## v0.1.0 (2026-11-16)

First release. Go port of the [gunsnprices](https://pypi.org/project/gunsnprices/) Python package 0.1.2, with
identical results on the 1,150 recorded store titles of its test suite and on all 63,755 titles of the GunsNPrices
catalog.

- `ParseTitle(title, *Options)`: firearm or ammo, brand, model, calibers, magazine capacity, barrel length, rounds per
  box, cost per round (`Options`: `Brand`, `MPN`, `SKU`, `Price`, `Kind`).
- `IdentifyModel`: 50 brands, the same model dictionaries that [GunsNPrices](https://gunsnprices.com) uses on its
  site; decodes Glock part numbers. `ListBrands`.
- `FindCalibers`, `FindCaliber`, `NormalizeCaliber`, `ListCalibers`, `CaliberByID`: 293 calibers with retailer
  spellings.
- `MagazineCapacity`, `BarrelLength`, `RoundsPerBox`, `CostPerRound`, `LooksLikeAmmo`.
- `ProductInfo` and `ModelMatch` marshal to JSON with the Python package's field names, field order and number format.
- `gunsnprices` command (`cmd/gunsnprices`): indented JSON for a title argument, JSON Lines for standard input.
- Standard library only; data embedded with `go:embed`; safe for concurrent use; Go 1.26+.
- The Python-syntax rules run on an internal backtracking engine with Python's `re` semantics (look-around,
  leftmost-first alternation, Unicode `\d` `\w` `\s` `\b`, `re.IGNORECASE`); Unicode tables from Python 3.13
  (Unicode 15.1).
