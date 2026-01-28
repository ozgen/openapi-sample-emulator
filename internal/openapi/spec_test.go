package openapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestLoadSpec_OpenAPI3_OK(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "oas3.json")

	specJSON := `{
	  "openapi":"3.0.3",
	  "info":{"title":"t","version":"1"},
	  "paths":{
		"/health":{
		  "get":{
			"responses":{
			  "200":{
				"description":"ok",
				"content":{
				  "application/json":{
					"example":{"ok":true}
				  }
				}
			  }
			}
		  }
		}
	  }
	}`

	if err := os.WriteFile(p, []byte(specJSON), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := LoadSpec(p)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if s == nil || s.Doc3 == nil {
		t.Fatalf("expected Doc3, got %#v", s)
	}
	if s.Doc2 != nil {
		t.Fatalf("expected Doc2 nil for OAS3, got %#v", s.Doc2)
	}

	op := findOperation(s, "/health", "get")
	if op == nil {
		t.Fatalf("expected operation")
	}
}

func TestLoadSpec_Swagger2_OK(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "swagger2.json")

	specJSON := `{
	  "swagger":"2.0",
	  "info":{"title":"t","version":"1"},
	  "basePath":"/",
	  "paths":{
		"/health":{
		  "get":{
			"responses":{
			  "200":{"description":"ok"}
			}
		  }
		}
	  }
	}`

	if err := os.WriteFile(p, []byte(specJSON), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := LoadSpec(p)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if s == nil || s.Doc2 == nil || s.Doc3 == nil {
		t.Fatalf("expected Doc2 and Doc3, got %#v", s)
	}

	if s.Doc3.Paths.Find("/health") == nil {
		t.Fatalf("expected /health in converted Doc3")
	}
}

func TestLoadSpec_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(p, []byte(`{ this is not json`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadSpec(p)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadSpec_MissingFile(t *testing.T) {
	_, err := LoadSpec("/no/such/file/openapi.json")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestPickBestResponseRef_Prefers200Then201202204(t *testing.T) {
	resps := openapi3.NewResponses()
	resps.Set("201", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("created")}})
	resps.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("ok")}})
	resps.Set("202", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("accepted")}})
	resps.Set("204", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("nocontent")}})

	got := pickBestResponseRef(resps)
	if got == nil || got.Value == nil || got.Value.Description == nil || *got.Value.Description != "ok" {
		t.Fatalf("expected 200, got %#v", got)
	}

	resps = openapi3.NewResponses()
	resps.Set("201", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("created")}})
	resps.Set("202", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("accepted")}})
	got = pickBestResponseRef(resps)
	if got == nil || got.Value == nil || got.Value.Description == nil || *got.Value.Description != "created" {
		t.Fatalf("expected 201, got %#v", got)
	}
}

func TestPickBestResponseRef_PicksLowest2xx(t *testing.T) {
	resps := openapi3.NewResponses()
	resps.Set("299", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("299")}})
	resps.Set("250", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("250")}})
	resps.Set("210", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("210")}})

	got := pickBestResponseRef(resps)
	if got == nil || got.Value == nil || got.Value.Description == nil || *got.Value.Description != "210" {
		t.Fatalf("expected lowest 2xx=210, got %#v", got)
	}
}

func TestExtractExampleFromResponse_Example(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"application/json": &openapi3.MediaType{Example: map[string]any{"id": 1}},
		},
	}

	b, ok := extractExampleFromResponse(resp)
	if !ok {
		t.Fatalf("expected ok")
	}

	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["id"] != float64(1) {
		t.Fatalf("unexpected: %#v", m)
	}
}

func TestExtractExampleFromResponse_ExamplesMap(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"application/problem+json": &openapi3.MediaType{
				Examples: openapi3.Examples{
					"ex1": &openapi3.ExampleRef{Value: &openapi3.Example{Value: map[string]any{"error": "x"}}},
				},
			},
		},
	}

	b, ok := extractExampleFromResponse(resp)
	if !ok {
		t.Fatalf("expected ok")
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["error"] != "x" {
		t.Fatalf("unexpected: %#v", m)
	}
}

func TestExtractExampleFromResponse_NoJSONContent(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"text/plain": &openapi3.MediaType{Example: "hi"},
		},
	}
	_, ok := extractExampleFromResponse(resp)
	if ok {
		t.Fatalf("expected false for non-json content types")
	}
}

func TestExtractExampleFromResponse_NilGuards(t *testing.T) {
	if _, ok := extractExampleFromResponse(nil); ok {
		t.Fatalf("expected false")
	}
	if _, ok := extractExampleFromResponse(&openapi3.Response{}); ok {
		t.Fatalf("expected false")
	}
}

func TestGenerateFromResponseSchema_JSON(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"application/json": &openapi3.MediaType{
				Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: openapi3.Schemas{
						"name": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					},
				}},
			},
		},
	}

	b, ok := generateFromResponseSchema(resp)
	if !ok {
		t.Fatalf("expected ok")
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["name"] != "string" {
		t.Fatalf("unexpected: %#v", m)
	}
}

func TestGenerateFromResponseSchema_ProblemJSON(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"application/problem+json": &openapi3.MediaType{
				Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
			},
		},
	}
	b, ok := generateFromResponseSchema(resp)
	if !ok {
		t.Fatalf("expected ok")
	}
	var s string
	_ = json.Unmarshal(b, &s)
	if s != "string" {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestGenerateFromResponseSchema_StarStar(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"*/*": &openapi3.MediaType{
				Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
			},
		},
	}
	b, ok := generateFromResponseSchema(resp)
	if !ok {
		t.Fatalf("expected ok")
	}
	var n any
	_ = json.Unmarshal(b, &n)
	if n != float64(0) {
		t.Fatalf("unexpected: %#v", n)
	}
}

func TestGenerateFromResponseSchema_NoSchema(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"application/json": &openapi3.MediaType{},
		},
	}
	_, ok := generateFromResponseSchema(resp)
	if ok {
		t.Fatalf("expected false")
	}
}

func TestGenerateFromResponseSchema_NilGuards(t *testing.T) {
	if _, ok := generateFromResponseSchema(nil); ok {
		t.Fatalf("expected false")
	}
	if _, ok := generateFromResponseSchema(&openapi3.Response{}); ok {
		t.Fatalf("expected false")
	}
}

func TestGenFromSchemaRef_EnumWins(t *testing.T) {
	v := genFromSchemaRef(&openapi3.SchemaRef{Value: &openapi3.Schema{
		Enum: []any{"a", "b"},
	}}, map[string]bool{}, 0)

	if v != "a" {
		t.Fatalf("expected first enum, got %#v", v)
	}
}

func TestGenFromSchemaRef_Primitives(t *testing.T) {
	tests := []struct {
		name string
		s    *openapi3.Schema
		want any
	}{
		{"string", &openapi3.Schema{Type: &openapi3.Types{"string"}}, "string"},
		{"datetime", &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "date-time"}, "2026-01-28T00:00:00Z"},
		{"integer", &openapi3.Schema{Type: &openapi3.Types{"integer"}}, 0},
		{"number", &openapi3.Schema{Type: &openapi3.Types{"number"}}, 0.0},
		{"boolean", &openapi3.Schema{Type: &openapi3.Types{"boolean"}}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := genFromSchemaRef(&openapi3.SchemaRef{Value: tc.s}, map[string]bool{}, 0)
			if got != tc.want {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestGenFromSchemaRef_Array(t *testing.T) {
	got := genFromSchemaRef(&openapi3.SchemaRef{Value: &openapi3.Schema{
		Type:  &openapi3.Types{"array"},
		Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
	}}, map[string]bool{}, 0)

	arr, ok := got.([]any)
	if !ok || len(arr) != 1 || arr[0] != "string" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestGenFromSchemaRef_ObjectProperties(t *testing.T) {
	got := genFromSchemaRef(&openapi3.SchemaRef{Value: &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: openapi3.Schemas{
			"id":   &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
			"name": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}}, map[string]bool{}, 0)

	m, ok := got.(map[string]any)
	if !ok || m["id"] != 0 || m["name"] != "string" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestGenObject_AdditionalPropertiesSchema(t *testing.T) {
	s := &openapi3.Schema{}
	s.AdditionalProperties.Schema = &openapi3.SchemaRef{
		Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
	}

	got := genObject(s, map[string]bool{}, 0)
	m, ok := got.(map[string]any)
	if !ok || m["key"] != "string" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestGenObject_AdditionalPropertiesTrue(t *testing.T) {
	s := &openapi3.Schema{
		Properties: openapi3.Schemas{
			"a": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
		},
	}
	b := true
	s.AdditionalProperties.Has = &b

	got := genObject(s, map[string]bool{}, 0)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("unexpected: %#v", got)
	}
	if m["a"] != 0 {
		t.Fatalf("expected property a, got %#v", m)
	}
	if m["key"] != "value" {
		t.Fatalf("expected additionalProperties placeholder, got %#v", m)
	}
}

func TestGenFromSchemaRef_DepthLimit(t *testing.T) {
	got := genFromSchemaRef(&openapi3.SchemaRef{Value: &openapi3.Schema{
		Type: &openapi3.Types{"object"},
	}}, map[string]bool{}, 7)
	m, ok := got.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty map due to depth limit, got %#v", got)
	}
}

func TestGenFromSchemaRef_NilGuards(t *testing.T) {
	got := genFromSchemaRef(nil, map[string]bool{}, 0)
	if _, ok := got.(map[string]any); !ok {
		t.Fatalf("expected map fallback, got %#v", got)
	}

	got = genFromSchemaRef(&openapi3.SchemaRef{Value: nil}, map[string]bool{}, 0)
	if _, ok := got.(map[string]any); !ok {
		t.Fatalf("expected map fallback, got %#v", got)
	}
}

func TestTryGetExampleBody_NoOperation(t *testing.T) {
	spec := &Spec{Doc3: &openapi3.T{}}
	_, ok := TryGetExampleBody(spec, "/missing", "get")
	if ok {
		t.Fatalf("expected false when operation not found or responses nil")
	}
}

func TestTryGetExampleBody_ResponseMissingValue_FallbackOK(t *testing.T) {
	paths := openapi3.NewPaths()
	paths.Set("/x", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Responses: func() *openapi3.Responses {
				r := openapi3.NewResponses()
				r.Set("200", &openapi3.ResponseRef{})
				return r
			}(),
		},
	})

	doc := &openapi3.T{}
	doc.Paths = paths

	spec := &Spec{Doc3: doc}
	b, ok := TryGetExampleBody(spec, "/x", "get")
	if !ok {
		t.Fatalf("expected ok")
	}

	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["ok"] != true {
		t.Fatalf("unexpected: %#v", m)
	}
}

func TestTryGetExampleBody_ExplicitExampleWins(t *testing.T) {
	paths := openapi3.NewPaths()
	paths.Set("/x", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Responses: func() *openapi3.Responses {
				r := openapi3.NewResponses()
				r.Set("200", &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Example: map[string]any{"hello": "world"},
							},
						},
					},
				})
				return r
			}(),
		},
	})

	doc := &openapi3.T{}
	doc.Paths = paths

	spec := &Spec{Doc3: doc}
	b, ok := TryGetExampleBody(spec, "/x", "get")
	if !ok {
		t.Fatalf("expected ok")
	}

	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["hello"] != "world" {
		t.Fatalf("unexpected: %#v", m)
	}
}

func TestTryGetExampleBody_SchemaGeneratedWhenNoExample(t *testing.T) {
	paths := openapi3.NewPaths()
	paths.Set("/x", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Responses: func() *openapi3.Responses {
				r := openapi3.NewResponses()
				r.Set("200", &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{
									Type: &openapi3.Types{"object"},
									Properties: openapi3.Schemas{
										"id": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}},
									},
								}},
							},
						},
					},
				})
				return r
			}(),
		},
	})

	doc := &openapi3.T{}
	doc.Paths = paths

	spec := &Spec{Doc3: doc}
	b, ok := TryGetExampleBody(spec, "/x", "get")
	if !ok {
		t.Fatalf("expected ok")
	}

	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["id"] != float64(0) {
		t.Fatalf("unexpected: %#v", m)
	}
}

func TestPickBestResponseRef_Prefers200(t *testing.T) {
	resps := openapi3.NewResponses()
	resps.Set("404", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("nope")}})
	resps.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("ok")}})

	got := pickBestResponseRef(resps)
	if got == nil || got.Value == nil || got.Value.Description == nil || *got.Value.Description != "ok" {
		t.Fatalf("expected 200 response, got %#v", got)
	}
}

func TestExtractExampleFromResponse_MediaTypeExample(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"application/json": &openapi3.MediaType{
				Example: map[string]any{"id": 1},
			},
		},
	}

	b, ok := extractExampleFromResponse(resp)
	if !ok {
		t.Fatalf("expected ok")
	}

	var m map[string]any
	_ = json.Unmarshal(b, &m)

	if m["id"] != float64(1) {
		t.Fatalf("unexpected: %#v", m)
	}
}

func TestGenerateFromResponseSchema_Array(t *testing.T) {
	resp := &openapi3.Response{
		Content: openapi3.Content{
			"application/json": &openapi3.MediaType{
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:  &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					},
				},
			},
		},
	}

	b, ok := generateFromResponseSchema(resp)
	if !ok {
		t.Fatalf("expected ok")
	}

	var v []any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(v) != 1 || v[0] != "string" {
		t.Fatalf("unexpected: %#v", v)
	}
}

func TestTryGetExampleBody_FallbackOk(t *testing.T) {
	paths := openapi3.NewPaths()
	paths.Set("/health", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Responses: openapi3.NewResponses(),
		},
	})

	doc := &openapi3.T{}
	doc.Paths = paths

	spec := &Spec{Doc3: doc}

	b, ok := TryGetExampleBody(spec, "/health", "get")
	if !ok {
		t.Fatalf("expected ok")
	}

	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["ok"] != true {
		t.Fatalf("unexpected: %#v", m)
	}
}

func ptr(s string) *string { return &s }
