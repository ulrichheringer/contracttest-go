// Package contracttest declares small HTTP contracts and validates JSON payloads.
package contracttest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// Type is a supported JSON type. The empty type accepts any JSON value.
type Type string

const (
	Object  Type = "object"
	Array   Type = "array"
	String  Type = "string"
	Number  Type = "number"
	Integer Type = "integer"
	Boolean Type = "boolean"
	Null    Type = "null"
)

// Schema is a deliberately small JSON Schema subset. Object properties are
// optional unless named in Required. Unknown object keys are allowed by default.
// A nil Items schema permits any array element. Nullable permits explicit null.
// Minimum and Maximum are inclusive. Schemas must not contain cycles.
type Schema struct {
	Type            Type
	Nullable        bool
	Properties      map[string]*Schema
	Required        []string
	DisallowUnknown bool
	Items           *Schema
	Minimum         *float64
	Maximum         *float64
}

// Validate checks the schema definition, including contradictory constraints.
func (s *Schema) Validate() error { return s.definition(make(map[*Schema]bool), 0) }

func (s *Schema) definition(seen map[*Schema]bool, depth int) error {
	if s == nil {
		return fmt.Errorf("nil schema")
	}
	if seen[s] || depth > 128 {
		return fmt.Errorf("cyclic or excessively nested schema")
	}
	seen[s] = true
	defer delete(seen, s)
	switch s.Type {
	case "", Object, Array, String, Number, Integer, Boolean, Null:
	default:
		return fmt.Errorf("unsupported type %q", s.Type)
	}
	if (len(s.Properties) > 0 || len(s.Required) > 0 || s.DisallowUnknown) && s.Type != Object {
		return fmt.Errorf("object constraints require object type")
	}
	if s.Items != nil && s.Type != Array {
		return fmt.Errorf("items require array type")
	}
	if (s.Minimum != nil || s.Maximum != nil) && s.Type != Number && s.Type != Integer {
		return fmt.Errorf("bounds require numeric type")
	}
	for _, bound := range []*float64{s.Minimum, s.Maximum} {
		if bound != nil && (math.IsNaN(*bound) || math.IsInf(*bound, 0)) {
			return fmt.Errorf("non-finite bound")
		}
	}
	if s.Minimum != nil && s.Maximum != nil && *s.Minimum > *s.Maximum {
		return fmt.Errorf("minimum exceeds maximum")
	}
	required := make(map[string]bool)
	for _, key := range s.Required {
		if required[key] {
			return fmt.Errorf("duplicate required property %q", key)
		}
		required[key] = true
		if _, ok := s.Properties[key]; !ok {
			return fmt.Errorf("required property %q has no schema", key)
		}
	}
	for _, key := range sortedKeys(s.Properties) {
		if err := s.Properties[key].definition(seen, depth+1); err != nil {
			return fmt.Errorf("property %q: %w", key, err)
		}
	}
	if s.Items != nil {
		return s.Items.definition(seen, depth+1)
	}
	return nil
}

// ValidateJSON validates exactly one JSON value. Numbers retain decimal precision.
func (s *Schema) ValidateJSON(body []byte) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return fmt.Errorf("JSON: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("JSON: expected exactly one value")
	}
	return s.value(value, "$")
}

func (s *Schema) value(v any, path string) error {
	if v == nil {
		if s.Nullable || s.Type == Null || s.Type == "" {
			return nil
		}
		return fmt.Errorf("%s: null is not allowed", path)
	}
	if s.Type == "" {
		return nil
	}
	switch s.Type {
	case Object:
		obj, ok := v.(map[string]any)
		if !ok {
			break
		}
		for _, key := range s.Required {
			if _, ok := obj[key]; !ok {
				return fmt.Errorf("%s: missing required property %q", path, key)
			}
		}
		for _, key := range sortedKeys(obj) {
			child, ok := s.Properties[key]
			if !ok {
				if s.DisallowUnknown {
					return fmt.Errorf("%s: unknown property %q", path, key)
				}
				continue
			}
			if err := child.value(obj[key], path+"["+strconv.Quote(key)+"]"); err != nil {
				return err
			}
		}
		return nil
	case Array:
		arr, ok := v.([]any)
		if !ok {
			break
		}
		if s.Items != nil {
			for i, item := range arr {
				if err := s.Items.value(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	case String:
		if _, ok := v.(string); ok {
			return nil
		}
	case Boolean:
		if _, ok := v.(bool); ok {
			return nil
		}
	case Number, Integer:
		n, ok := v.(json.Number)
		if !ok {
			break
		}
		// Bound rational allocation for extreme exponents in malformed fixtures.
		if len(n) > 10000 {
			return fmt.Errorf("%s: number exceeds 10000 digits", path)
		}
		if pos := strings.IndexAny(string(n), "eE"); pos >= 0 {
			exponent, err := strconv.ParseInt(string(n)[pos+1:], 10, 32)
			if err != nil || exponent < -10000 || exponent > 10000 {
				return fmt.Errorf("%s: exponent exceeds supported range", path)
			}
		}
		rat, ok := new(big.Rat).SetString(string(n))
		if !ok {
			return fmt.Errorf("%s: invalid number", path)
		}
		if s.Type == Integer && !rat.IsInt() {
			return fmt.Errorf("%s: expected integer", path)
		}
		for i, bound := range []*float64{s.Minimum, s.Maximum} {
			if bound == nil {
				continue
			}
			b, _ := new(big.Rat).SetString(strconv.FormatFloat(*bound, 'g', -1, 64))
			cmp := rat.Cmp(b)
			if (i == 0 && cmp < 0) || (i == 1 && cmp > 0) {
				return fmt.Errorf("%s: numeric bound violated", path)
			}
		}
		return nil
	}
	return fmt.Errorf("%s: expected %s", path, s.Type)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
