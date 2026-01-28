package openapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Spec supports both Swagger 2.0 and (optionally) OpenAPI 3.x.
type Spec struct {
	Swagger     string                    `json:"swagger"` // "2.0"
	OpenAPI     string                    `json:"openapi"` // "3.x"
	Produces    []string                  `json:"produces"`
	Paths       map[string]map[string]any `json:"paths"`
	Definitions map[string]map[string]any `json:"definitions"` // Swagger 2.0
	Components  map[string]any            `json:"components"`
	// OpenAPI 3.x (not required for Swagger2)
}

// LoadSpec reads swagger.json/openapi.json and parses minimal fields we need.
func LoadSpec(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	var s Spec
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse spec json: %w", err)
	}

	if s.Paths == nil {
		s.Paths = map[string]map[string]any{}
	}
	if s.Definitions == nil {
		s.Definitions = map[string]map[string]any{}
	}

	return &s, nil
}

func TryGetExampleBody(spec *Spec, swaggerPath, method string) ([]byte, bool) {
	if spec == nil || spec.Paths == nil {
		return nil, false
	}

	m := spec.Paths[swaggerPath]
	if m == nil {
		return nil, false
	}

	method = strings.ToLower(method)

	opAny, ok := m[method]
	if !ok || opAny == nil {
		return nil, false
	}

	op, ok := opAny.(map[string]any)
	if !ok {
		return nil, false
	}

	responsesAny, ok := op["responses"]
	if !ok || responsesAny == nil {
		return nil, false
	}

	responses, ok := responsesAny.(map[string]any)
	if !ok || len(responses) == 0 {
		return nil, false
	}

	respObj := pickBestResponse(responses)

	if body, ok := extractOAS3Example(respObj); ok {
		return body, true
	}

	if body, ok := extractSwagger2Example(respObj); ok {
		return body, true
	}

	schemaAny, ok := respObj["schema"]
	if !ok || schemaAny == nil {
		b, _ := json.Marshal(map[string]any{"ok": true})
		return b, true
	}

	schema, ok := schemaAny.(map[string]any)
	if !ok {
		b, _ := json.Marshal(map[string]any{"ok": true})
		return b, true
	}

	val := generateFromSchema(spec, schema, map[string]bool{}, 0)
	b, err := json.Marshal(val)
	if err != nil {
		return nil, false
	}
	return b, true
}

func pickBestResponse(responses map[string]any) map[string]any {
	for _, code := range []string{"200", "201", "202", "204"} {
		if rAny, ok := responses[code]; ok && rAny != nil {
			if r, ok := rAny.(map[string]any); ok {
				return r
			}
		}
	}

	for code, rAny := range responses {
		if rAny == nil {
			continue
		}
		if strings.HasPrefix(code, "2") {
			if r, ok := rAny.(map[string]any); ok {
				return r
			}
		}
	}

	if rAny, ok := responses["default"]; ok && rAny != nil {
		if r, ok := rAny.(map[string]any); ok {
			return r
		}
	}

	for _, rAny := range responses {
		if rAny == nil {
			continue
		}
		if r, ok := rAny.(map[string]any); ok {
			return r
		}
	}

	return map[string]any{}
}

func extractSwagger2Example(resp map[string]any) ([]byte, bool) {
	exAny, ok := resp["examples"]
	if !ok || exAny == nil {
		return nil, false
	}
	examples, ok := exAny.(map[string]any)
	if !ok || len(examples) == 0 {
		return nil, false
	}

	for _, ct := range []string{"application/json", "application/problem+json", "*/*"} {
		if v, ok := examples[ct]; ok && v != nil {
			b, err := json.Marshal(v)
			return b, err == nil
		}
	}
	for _, v := range examples {
		if v != nil {
			b, err := json.Marshal(v)
			return b, err == nil
		}
	}
	return nil, false
}

func extractOAS3Example(resp map[string]any) ([]byte, bool) {
	contentAny, ok := resp["content"]
	if !ok || contentAny == nil {
		return nil, false
	}
	content, ok := contentAny.(map[string]any)
	if !ok || len(content) == 0 {
		return nil, false
	}

	for _, ct := range []string{"application/json", "application/problem+json", "*/*"} {
		appAny, ok := content[ct]
		if !ok || appAny == nil {
			continue
		}
		app, ok := appAny.(map[string]any)
		if !ok {
			continue
		}

		if ex, ok := app["example"]; ok && ex != nil {
			b, err := json.Marshal(ex)
			return b, err == nil
		}

		if exsAny, ok := app["examples"]; ok && exsAny != nil {
			if exs, ok := exsAny.(map[string]any); ok {
				for _, v := range exs {
					if exObj, ok := v.(map[string]any); ok && exObj != nil {
						if val, ok := exObj["value"]; ok && val != nil {
							b, err := json.Marshal(val)
							return b, err == nil
						}
					}
				}
			}
		}
	}

	return nil, false
}

func generateFromSchema(spec *Spec, schema map[string]any, visiting map[string]bool, depth int) any {
	if depth > 6 {
		return map[string]any{}
	}

	if refAny, ok := schema["$ref"]; ok && refAny != nil {
		ref, _ := refAny.(string)
		name := strings.TrimPrefix(ref, "#/definitions/")
		if name == "" {
			return map[string]any{}
		}
		if visiting[name] {
			return map[string]any{}
		}
		def, ok := spec.Definitions[name]
		if !ok || def == nil {
			return map[string]any{}
		}
		visiting[name] = true
		val := generateFromSchema(spec, def, visiting, depth+1)
		delete(visiting, name)
		return val
	}

	t, _ := schema["type"].(string)

	switch t {
	case "string":
		if enumAny, ok := schema["enum"]; ok && enumAny != nil {
			if arr, ok := enumAny.([]any); ok && len(arr) > 0 {
				if s, ok := arr[0].(string); ok {
					return s
				}
			}
		}
		if format, _ := schema["format"].(string); format == "date-time" {
			return "2026-01-28T00:00:00Z"
		}
		return "string"

	case "integer":
		return 0
	case "number":
		return 0.0
	case "boolean":
		return true

	case "array":
		itemsAny, ok := schema["items"]
		if !ok || itemsAny == nil {
			return []any{}
		}
		items, ok := itemsAny.(map[string]any)
		if !ok {
			return []any{}
		}
		return []any{generateFromSchema(spec, items, visiting, depth+1)}

	case "object":
		out := map[string]any{}

		if apAny, ok := schema["additionalProperties"]; ok && apAny != nil {
			if ap, ok := apAny.(map[string]any); ok {
				out["key"] = generateFromSchema(spec, ap, visiting, depth+1)
				return out
			}
		}

		propsAny, ok := schema["properties"]
		if !ok || propsAny == nil {
			return out
		}
		props, ok := propsAny.(map[string]any)
		if !ok {
			return out
		}

		for k, v := range props {
			propSchema, ok := v.(map[string]any)
			if !ok {
				continue
			}
			out[k] = generateFromSchema(spec, propSchema, visiting, depth+1)
		}
		return out

	default:
		if _, ok := schema["properties"]; ok {
			schema["type"] = "object"
			return generateFromSchema(spec, schema, visiting, depth+1)
		}
		return map[string]any{"ok": true}
	}
}
