package contracttest

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ExportOpenAPI writes an OpenAPI 3.1 document for the supported contract subset.
// Duplicate method/path operations are rejected rather than silently overwritten.
// A nil body schema means unspecified content. It does not imply an empty body.
// This is an exporter, not a general OpenAPI importer or validator.
func ExportOpenAPI(w io.Writer, title, version string, contracts ...Contract) error {
	if w == nil || title == "" || version == "" {
		return fmt.Errorf("writer, title and version are required")
	}
	paths := map[string]map[string]any{}
	for _, c := range contracts {
		if err := c.Validate(); err != nil {
			return err
		}
		method := strings.ToLower(c.Method)
		if paths[c.Path] == nil {
			paths[c.Path] = map[string]any{}
		}
		if _, ok := paths[c.Path][method]; ok {
			return fmt.Errorf("duplicate operation %s %s", c.Method, c.Path)
		}
		if c.Method == "CONNECT" {
			return fmt.Errorf("CONNECT is not an OpenAPI path operation")
		}
		op := map[string]any{}
		response := map[string]any{"description": httpDescription(c.Status)}
		if c.ResponseBody != nil {
			response["content"] = content(c.ResponseBody)
		}
		if len(c.ResponseHeaders) > 0 {
			headers := map[string]any{}
			for name, values := range c.ResponseHeaders {
				if strings.EqualFold(name, "Content-Type") {
					return fmt.Errorf("OpenAPI ignores Content-Type response headers; use JSON content")
				}
				if len(values) != 1 {
					return fmt.Errorf("OpenAPI export requires single-valued header %q", name)
				}
				headers[name] = map[string]any{"required": true, "schema": map[string]any{"type": "string", "const": values[0]}}
			}
			response["headers"] = headers
		}
		op["responses"] = map[string]any{strconv.Itoa(c.Status): response}
		if c.RequestBody != nil {
			op["requestBody"] = map[string]any{"required": true, "content": content(c.RequestBody)}
		}
		params := []any{}
		parts, _ := pathParts(c.Path)
		for _, p := range parts {
			if strings.HasPrefix(p, "{") {
				params = append(params, map[string]any{"in": "path", "name": p[1 : len(p)-1], "required": true, "schema": map[string]any{"type": "string", "minLength": 1}})
			}
		}
		for _, name := range sortedKeys(c.RequestHeaders) {
			if strings.EqualFold(name, "Content-Type") || strings.EqualFold(name, "Accept") || strings.EqualFold(name, "Authorization") {
				return fmt.Errorf("OpenAPI ignores header parameter %q; omit it from export", name)
			}
			values := c.RequestHeaders[name]
			if len(values) != 1 {
				return fmt.Errorf("OpenAPI export requires single-valued header %q", name)
			}
			params = append(params, map[string]any{"in": "header", "name": name, "required": true, "schema": map[string]any{"type": "string", "const": values[0]}})
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		paths[c.Path][method] = op
	}
	doc := map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": title, "version": version}, "paths": paths}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(doc)
}

func httpDescription(status int) string { return fmt.Sprintf("HTTP %d response", status) }
func content(s *Schema) map[string]any {
	return map[string]any{"application/json": map[string]any{"schema": s.openapi()}}
}
func (s *Schema) openapi() map[string]any {
	out := map[string]any{}
	if s.Type != "" {
		if s.Nullable && s.Type != Null {
			out["type"] = []Type{s.Type, Null}
		} else {
			out["type"] = s.Type
		}
	}
	if s.Type == Object {
		props := map[string]any{}
		for k, v := range s.Properties {
			props[k] = v.openapi()
		}
		out["properties"] = props
		out["additionalProperties"] = !s.DisallowUnknown
		if len(s.Required) > 0 {
			out["required"] = s.Required
		}
	}
	if s.Items != nil {
		out["items"] = s.Items.openapi()
	}
	if s.Minimum != nil {
		out["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		out["maximum"] = *s.Maximum
	}
	return out
}
