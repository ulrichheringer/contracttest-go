package contracttest

import (
	"strings"
	"testing"
)

func ptr(v float64) *float64 { return &v }

func TestJSONValidation(t *testing.T) {
	s := &Schema{Type: Object, Required: []string{"users"}, DisallowUnknown: true, Properties: map[string]*Schema{"users": {Type: Array, Items: &Schema{Type: Object, Required: []string{"id"}, Properties: map[string]*Schema{"id": {Type: Integer, Minimum: ptr(1)}, "name": {Type: String, Nullable: true}}}}}}
	for _, tt := range []struct{ name, body, want string }{
		{"nested", `{"users":[{"id":9007199254740993,"name":null}]}`, ""},
		{"exponent integer", `{"users":[{"id":1e3}]}`, ""},
		{"missing", `{}`, "missing required"},
		{"unknown", `{"users":[],"other":1}`, "unknown property"},
		{"null required", `{"users":null}`, "null is not allowed"},
		{"fraction", `{"users":[{"id":1.0000000000000001}]}`, "expected integer"},
		{"nested missing", `{"users":[{}]}`, "missing required"},
		{"minimum", `{"users":[{"id":0}]}`, "numeric bound"},
		{"type", `{"users":false}`, "expected array"},
		{"trailing", `{"users":[]} true`, "exactly one"},
		{"invalid", `{`, "JSON"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := s.ValidateJSON([]byte(tt.body))
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %s", err, tt.want)
			}
		})
	}
}

func TestSchemaDefinitions(t *testing.T) {
	cycle := &Schema{Type: Array}
	cycle.Items = cycle
	for _, s := range []*Schema{nil, cycle, {Type: "bogus"}, {Type: String, Minimum: ptr(1)}, {Type: Number, Minimum: ptr(2), Maximum: ptr(1)}, {Type: Object, Required: []string{"missing"}}, {Type: Object, Properties: map[string]*Schema{"nil": nil}}, {Type: String, Items: &Schema{}}} {
		if err := s.Validate(); err == nil {
			t.Fatalf("expected invalid schema %#v", s)
		}
	}
}

func TestPrimitiveTypesAndBounds(t *testing.T) {
	for _, tt := range []struct {
		s     Schema
		body  string
		valid bool
	}{
		{Schema{Type: Boolean}, `true`, true}, {Schema{Type: Boolean}, `1`, false},
		{Schema{Type: Null}, `null`, true}, {Schema{Type: Null}, `false`, false},
		{Schema{}, `{"anything":[1,null]}`, true}, {Schema{Type: String}, `"hi"`, true},
		{Schema{Type: Number, Minimum: ptr(.1), Maximum: ptr(.3)}, `.1`, false},
		{Schema{Type: Number, Minimum: ptr(.1), Maximum: ptr(.3)}, `0.1`, true},
		{Schema{Type: Number, Maximum: ptr(.3)}, `0.30000000000000001`, false},
	} {
		err := tt.s.ValidateJSON([]byte(tt.body))
		if (err == nil) != tt.valid {
			t.Errorf("%s: %v", tt.body, err)
		}
	}
}
