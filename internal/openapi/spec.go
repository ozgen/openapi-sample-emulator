package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
)

type Spec struct {
	Doc3 *openapi3.T
	Doc2 *openapi2.T
}

type versionProbe struct {
	Swagger string `json:"swagger"`
	OpenAPI string `json:"openapi"`
}

func LoadSpec(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	var probe versionProbe
	_ = json.Unmarshal(b, &probe)

	abs, _ := filepath.Abs(path)
	loc := &url.URL{Scheme: "file", Path: abs}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	// Swagger 2.0
	if probe.Swagger == "2.0" {
		var doc2 openapi2.T
		if err := json.Unmarshal(b, &doc2); err != nil {
			return nil, fmt.Errorf("parse swagger2 json: %w", err)
		}

		doc3, err := openapi2conv.ToV3WithLoader(&doc2, loader, loc)
		if err != nil {
			return nil, fmt.Errorf("convert swagger2 -> oas3: %w", err)
		}

		if err := loader.ResolveRefsIn(doc3, loc); err != nil {
			return nil, fmt.Errorf("resolve refs: %w", err)
		}

		_ = doc3.Validate(context.Background())

		return &Spec{Doc2: &doc2, Doc3: doc3}, nil
	}

	// OpenAPI 3.x
	var doc3 openapi3.T
	if err := json.Unmarshal(b, &doc3); err != nil {
		return nil, fmt.Errorf("parse openapi3 json: %w", err)
	}
	if err := loader.ResolveRefsIn(&doc3, loc); err != nil {
		return nil, fmt.Errorf("resolve refs: %w", err)
	}
	_ = doc3.Validate(context.Background())

	return &Spec{Doc3: &doc3}, nil
}

func TryGetExampleBody(spec *Spec, swaggerPath, method string) ([]byte, bool) {
	op := findOperation(spec, swaggerPath, method)
	if op == nil || op.Responses == nil {
		return nil, false
	}

	respRef := pickBestResponseRef(op.Responses)
	if respRef == nil || respRef.Value == nil {
		b, _ := json.Marshal(map[string]any{"ok": true})
		return b, true
	}

	if b, ok := extractExampleFromResponse(respRef.Value); ok {
		return b, true
	}

	if b, ok := generateFromResponseSchema(respRef.Value); ok {
		return b, true
	}

	b, _ := json.Marshal(map[string]any{"ok": true})
	return b, true
}

func findOperation(spec *Spec, swaggerPath, method string) *openapi3.Operation {
	if spec == nil || spec.Doc3 == nil {
		return nil
	}
	item := spec.Doc3.Paths.Find(swaggerPath)
	if item == nil {
		return nil
	}
	return item.GetOperation(strings.ToUpper(method))
}

func pickBestResponseRef(resps *openapi3.Responses) *openapi3.ResponseRef {
	if resps == nil {
		return nil
	}

	// Prefer 200/201/202/204 if present
	for _, code := range []string{"200", "201", "202", "204"} {
		if r := resps.Value(code); r != nil {
			return r
		}
	}

	// Then any 2xx (lowest first)
	var twos []int
	for k := range resps.Map() {
		if n, err := strconv.Atoi(k); err == nil && n >= 200 && n < 300 {
			twos = append(twos, n)
		}
	}
	sort.Ints(twos)
	for _, n := range twos {
		if r := resps.Value(strconv.Itoa(n)); r != nil {
			return r
		}
	}

	// Then default
	if r := resps.Value("default"); r != nil {
		return r
	}

	// Otherwise: return any
	for _, r := range resps.Map() {
		if r != nil {
			return r
		}
	}
	return nil
}

func extractExampleFromResponse(resp *openapi3.Response) ([]byte, bool) {
	if resp == nil || resp.Content == nil {
		return nil, false
	}

	// Try common JSON-like content types
	for _, ct := range []string{"application/json", "application/problem+json", "*/*"} {
		mt := resp.Content.Get(ct)
		if mt == nil {
			continue
		}

		// MediaType.Example
		if mt.Example != nil {
			if b, err := json.Marshal(mt.Example); err == nil {
				return b, true
			}
		}

		if len(mt.Examples) > 0 {
			for _, exRef := range mt.Examples {
				if exRef == nil || exRef.Value == nil {
					continue
				}
				if exRef.Value.Value != nil {
					if b, err := json.Marshal(exRef.Value.Value); err == nil {
						return b, true
					}
				}
			}
		}
	}

	return nil, false
}

func generateFromResponseSchema(resp *openapi3.Response) ([]byte, bool) {
	if resp == nil || resp.Content == nil {
		return nil, false
	}

	for _, ct := range []string{"application/json", "application/problem+json", "*/*"} {
		mt := resp.Content.Get(ct)
		if mt == nil || mt.Schema == nil {
			continue
		}

		val := genFromSchemaRef(mt.Schema, map[string]bool{}, 0)
		b, err := json.Marshal(val)
		return b, err == nil
	}

	return nil, false
}

func genFromSchemaRef(ref *openapi3.SchemaRef, visiting map[string]bool, depth int) any {
	if depth > 6 || ref == nil || ref.Value == nil {
		return map[string]any{}
	}

	s := ref.Value

	// enum wins
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}

	// ARRAY
	if s.Type != nil && s.Type.Is("array") {
		if s.Items == nil {
			return []any{}
		}
		return []any{genFromSchemaRef(s.Items, visiting, depth+1)}
	}

	// OBJECT
	if (s.Type != nil && s.Type.Is("object")) || len(s.Properties) > 0 || s.AdditionalProperties.Schema != nil {
		return genObject(s, visiting, depth)
	}

	// PRIMITIVES
	if s.Type != nil && s.Type.Is("string") {
		if s.Format == "date-time" {
			return "2026-01-28T00:00:00Z"
		}
		return "string"
	}
	if s.Type != nil && s.Type.Is("integer") {
		return 0
	}
	if s.Type != nil && s.Type.Is("number") {
		return 0.0
	}
	if s.Type != nil && s.Type.Is("boolean") {
		return true
	}

	// fallback
	return map[string]any{"ok": true}
}

func genObject(s *openapi3.Schema, visiting map[string]bool, depth int) any {
	out := map[string]any{}

	// additionalProperties: schema form
	if s.AdditionalProperties.Schema != nil {
		out["key"] = genFromSchemaRef(s.AdditionalProperties.Schema, visiting, depth+1)
		return out
	}

	if s.AdditionalProperties.Has != nil && *s.AdditionalProperties.Has {
		out["key"] = "value"
	}

	// properties
	for name, prop := range s.Properties {
		out[name] = genFromSchemaRef(prop, visiting, depth+1)
	}

	return out
}
