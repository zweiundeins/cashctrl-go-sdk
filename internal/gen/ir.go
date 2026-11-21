package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// The types below read the TypeScript SDK's spec files as its scripts/ir.ts
// defines them.

// Param is one documented request parameter.
type Param struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // TEXT NUMBER BOOLEAN JSON CSV DATE HTML XML
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	MaxLength   int      `json:"maxLength,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     string   `json:"default,omitempty"`
	Fields      []Param  `json:"fields,omitempty"`
	IsArray     bool     `json:"isArray,omitempty"`
}

// Endpoint is one documented endpoint.
type Endpoint struct {
	Anchor      string   `json:"anchor"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Group       []string `json:"group"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Params      []Param  `json:"params"`
}

// Spec is spec/api.json.
type Spec struct {
	Source          string     `json:"source"`
	BaseURLTemplate string     `json:"baseUrlTemplate"`
	Endpoints       []Endpoint `json:"endpoints"`
}

// Shape is a response shape the TypeScript SDK's probe inferred.
type Shape struct {
	Kind   string // scalar self object array unknown
	Types  []string
	Fields []ShapeField // in the order the probe saw them
	Items  *Shape
	// Unobserved marks a scalar typed from the parameter docs because every
	// sample was null; a number then decodes tolerantly (NullInt64, NullFloat64).
	Unobserved bool
}

// ShapeField is one member of an object shape.
type ShapeField struct {
	Name     string
	Shape    Shape
	Optional bool
}

func (s Shape) field(name string) (ShapeField, bool) {
	for _, f := range s.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return ShapeField{}, false
}

// UnmarshalJSON keeps the order of an object's fields, which the generated
// structs follow.
func (s *Shape) UnmarshalJSON(b []byte) error {
	var raw struct {
		Kind   string          `json:"kind"`
		Types  []string        `json:"types"`
		Fields json.RawMessage `json:"fields"`
		Items  *Shape          `json:"items"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*s = Shape{Kind: raw.Kind, Types: raw.Types, Items: raw.Items}
	if len(raw.Fields) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw.Fields))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("shape fields: want an object")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		var f struct {
			Shape    Shape `json:"shape"`
			Optional bool  `json:"optional"`
		}
		if err := dec.Decode(&f); err != nil {
			return err
		}
		s.Fields = append(s.Fields, ShapeField{Name: t.(string), Shape: f.Shape, Optional: f.Optional})
	}
	return nil
}

// InferredResponse is one probed endpoint.
type InferredResponse struct {
	Envelope string `json:"envelope"` // data write raw
	Shape    Shape  `json:"shape"`
	Samples  int    `json:"samples"`
	Error    string `json:"error,omitempty"`
}

// ResponseSpec is spec/responses.json.
type ResponseSpec struct {
	ProbedAt     string                      `json:"probedAt"`
	Organisation string                      `json:"organisation"`
	Responses    map[string]InferredResponse `json:"responses"`
}

func (r ResponseSpec) ok(path string) (InferredResponse, bool) {
	p, found := r.Responses[path]
	return p, found && p.Error == ""
}
