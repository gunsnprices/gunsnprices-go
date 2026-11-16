package gunsnprices

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
)

// The model dictionaries and the caliber list, the same files as the Python package (CC BY 4.0, see LICENSE-DATA).

//go:embed data/models.json
var modelsJSON []byte

//go:embed data/calibers.json
var calibersJSON []byte

// object is a JSON object that keeps its key order (the rules depend on it, as in Python, where dicts are ordered).
type object struct {
	keys []string
	vals map[string]any
}

func (o *object) get(k string) any {
	if o == nil {
		return nil
	}
	return o.vals[k]
}

func (o *object) str(k string) string {
	s, _ := o.get(k).(string)
	return s
}

func (o *object) obj(k string) *object {
	v, _ := o.get(k).(*object)
	return v
}

func (o *object) list(k string) []any {
	v, _ := o.get(k).([]any)
	return v
}

// strings returns a list of strings (nil if the key is missing or null).
func (o *object) strings(k string) []string {
	l := o.list(k)
	out := make([]string, 0, len(l))
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// pairs returns the string values of an object in key order.
func (o *object) pairs(k string) [][2]string {
	m := o.obj(k)
	if m == nil {
		return nil
	}
	out := make([][2]string, 0, len(m.keys))
	for _, key := range m.keys {
		s, _ := m.vals[key].(string)
		out = append(out, [2]string{key, s})
	}
	return out
}

// stringMap returns an object of strings as a map.
func (o *object) stringMap(k string) map[string]string {
	out := map[string]string{}
	for _, p := range o.pairs(k) {
		out[p[0]] = p[1]
	}
	return out
}

func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err == nil {
		return nil, fmt.Errorf("trailing data")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := &object{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("object key %v", kt)
				}
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				if _, dup := o.vals[key]; !dup {
					o.keys = append(o.keys, key)
				}
				o.vals[key] = v
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return o, nil
		case '[':
			l := []any{}
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				l = append(l, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return l, nil
		}
		return nil, fmt.Errorf("unexpected %v", t)
	default:
		return tok, nil
	}
}

func mustObject(data []byte, name string) *object {
	v, err := decodeJSON(data)
	if err != nil {
		panic(fmt.Sprintf("gunsnprices: bad embedded %s: %v", name, err))
	}
	o, ok := v.(*object)
	if !ok {
		panic(fmt.Sprintf("gunsnprices: bad embedded %s", name))
	}
	return o
}

// number returns a JSON integer (0 if missing).
func number(v any) int {
	n, ok := v.(json.Number)
	if !ok {
		return 0
	}
	i, err := n.Int64()
	if err != nil {
		f, _ := n.Float64()
		return int(f)
	}
	return int(i)
}
