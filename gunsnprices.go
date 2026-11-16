package gunsnprices

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Version is the version of this module.
const Version = "0.1.0"

// UnicodeVersion is the Unicode version of the character tables the parser uses (those of Python 3.13, so results
// do not depend on the Go version).
const UnicodeVersion = "15.1.0"

// Kind tells firearms from ammunition.
type Kind string

const (
	// Firearm is a gun (or a magazine, part or accessory: anything that is not ammunition).
	Firearm Kind = "firearm"
	// Ammo is ammunition.
	Ammo Kind = "ammo"
)

// Options are the optional inputs of ParseTitle and IdentifyModel. The zero value means "not known".
type Options struct {
	// Brand is the manufacturer, if known (the manufacturer field of a product feed). It helps with titles that do
	// not name the brand.
	Brand string
	// MPN is the manufacturer part number, if known. Glock part numbers encode the model and the generation.
	MPN string
	// SKU is the store SKU, if known.
	SKU string
	// Price is the price of the listing; with ammunition it gives CostPerRound. 0 means unknown.
	Price float64
	// Kind skips the firearm/ammo guess when it is Firearm or Ammo.
	Kind Kind
}

// ProductInfo is the result of ParseTitle. Fields that do not apply or were not found are nil (or empty).
type ProductInfo struct {
	// Title is the title that was parsed.
	Title string `json:"title"`
	// Kind is Firearm or Ammo.
	Kind Kind `json:"kind"`
	// Brand is the brand of the identified model, else Options.Brand, else "".
	Brand string `json:"brand"`
	// Model is the identified model (firearms only).
	Model *ModelMatch `json:"model"`
	// Caliber is the first caliber in the title.
	Caliber *Caliber `json:"caliber"`
	// Calibers are all calibers in the title, in order (".357 Mag / .38 Special" gives two).
	Calibers []Caliber `json:"calibers"`
	// Capacity is the magazine capacity (firearms only; for "15+1" it is 15).
	Capacity *int `json:"capacity"`
	// BarrelLength is the barrel length in inches (firearms only).
	BarrelLength *float64 `json:"barrel_length"`
	// Rounds is the number of rounds per box (ammunition only).
	Rounds *int `json:"rounds"`
	// CostPerRound is the price of one round (ammunition with Options.Price only).
	CostPerRound *float64 `json:"cost_per_round"`
}

// ParseTitle parses a product title as a store shows it into brand, model, caliber, magazine capacity, barrel
// length, rounds per box and cost per round. opts may be nil.
//
// Unless opts.Kind says otherwise, a title that reads like ammunition (bullet weight, "ammo", "50/box", ...) is
// parsed as ammunition: calibers, rounds per box and cost per round. Anything else is parsed as a firearm: model,
// calibers, magazine capacity and barrel length.
func ParseTitle(title string, opts *Options) ProductInfo {
	var o Options
	if opts != nil {
		o = *opts
	}
	title, brand := clean(title), clean(o.Brand)
	kind := o.Kind
	if kind != Firearm && kind != Ammo {
		kind = Firearm
		if LooksLikeAmmo(title) {
			kind = Ammo
		}
	}
	info := ProductInfo{Title: title, Kind: kind, Brand: brand, Calibers: FindCalibers(title)}
	if len(info.Calibers) > 0 {
		c := info.Calibers[0]
		info.Caliber = &c
	}
	if kind == Ammo {
		if n, ok := RoundsPerBox(title); ok {
			info.Rounds = &n
			if o.Price != 0 {
				if c, ok := CostPerRound(o.Price, n); ok {
					info.CostPerRound = &c
				}
			}
		}
		return info
	}
	if m := identifyModel(title, brand, clean(o.MPN), clean(o.SKU)); m != nil {
		info.Model = m
		info.Brand = m.Brand
	}
	if n, ok := MagazineCapacity(title); ok {
		info.Capacity = &n
	}
	if v, ok := BarrelLength(title); ok {
		info.BarrelLength = &v
	}
	return info
}

// jsonFloat formats a float the way Python's json module does (repr: "26.0", "0.45", "1e-05").
type jsonFloat float64

func (f jsonFloat) MarshalJSON() ([]byte, error) {
	x := float64(f)
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return []byte("null"), nil
	}
	return []byte(pyRepr(x)), nil
}

// pyRepr is Python's repr() of a finite float: the shortest round-trip digits, scientific notation when the decimal
// exponent is below -4 or at least 16, and ".0" on integral values.
func pyRepr(x float64) string {
	e := strconv.FormatFloat(x, 'e', -1, 64)
	exp, _ := strconv.Atoi(e[strings.IndexByte(e, 'e')+1:])
	if exp < -4 || exp >= 16 {
		return e
	}
	s := strconv.FormatFloat(x, 'f', -1, 64)
	if !strings.ContainsAny(s, ".") {
		s += ".0"
	}
	return s
}

// MarshalJSON gives the same fields, in the same order and with the same number formatting, as the Python
// package's ProductInfo.to_dict() serialized with json.dumps.
func (p ProductInfo) MarshalJSON() ([]byte, error) {
	type out struct {
		Title        string      `json:"title"`
		Kind         Kind        `json:"kind"`
		Brand        string      `json:"brand"`
		Model        *ModelMatch `json:"model"`
		Caliber      *Caliber    `json:"caliber"`
		Calibers     []Caliber   `json:"calibers"`
		Capacity     *int        `json:"capacity"`
		BarrelLength *jsonFloat  `json:"barrel_length"`
		Rounds       *int        `json:"rounds"`
		CostPerRound *jsonFloat  `json:"cost_per_round"`
	}
	o := out{Title: p.Title, Kind: p.Kind, Brand: p.Brand, Model: p.Model, Caliber: p.Caliber, Calibers: p.Calibers,
		Capacity: p.Capacity, Rounds: p.Rounds}
	if o.Calibers == nil {
		o.Calibers = []Caliber{}
	}
	if p.BarrelLength != nil {
		v := jsonFloat(*p.BarrelLength)
		o.BarrelLength = &v
	}
	if p.CostPerRound != nil {
		v := jsonFloat(*p.CostPerRound)
		o.CostPerRound = &v
	}
	return marshalNoHTML(o)
}

// marshalNoHTML is json.Marshal without escaping "<", ">" and "&": json.Marshal escapes them again in the output
// of a MarshalJSON method, and an Encoder with SetEscapeHTML(false) leaves them as they are.
func marshalNoHTML(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// MarshalJSON writes nil Configs and Features as [] like the Python package.
func (m ModelMatch) MarshalJSON() ([]byte, error) {
	type plain ModelMatch
	p := plain(m)
	if p.Configs == nil {
		p.Configs = []string{}
	}
	if p.Features == nil {
		p.Features = []string{}
	}
	return marshalNoHTML(p)
}
