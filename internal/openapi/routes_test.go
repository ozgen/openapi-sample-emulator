package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestSwaggerPathToRegex_Static(t *testing.T) {
	re := swaggerPathToRegex("/v1/health")

	ok := []string{"/v1/health", "/v1/health/"}
	for _, p := range ok {
		if !re.MatchString(p) {
			t.Fatalf("expected %q to match %q", re.String(), p)
		}
	}

	bad := []string{"/v1/health/x", "/v1/healt", "/v1/health//"}
	for _, p := range bad {
		if re.MatchString(p) {
			t.Fatalf("expected %q to NOT match %q", re.String(), p)
		}
	}
}

func TestSwaggerPathToRegex_Param(t *testing.T) {
	re := swaggerPathToRegex("/users/{id}")

	if !re.MatchString("/users/123") {
		t.Fatalf("expected match")
	}
	if re.MatchString("/users/123/profile") {
		t.Fatalf("expected no match")
	}
}

func TestSwaggerPathToSampleName(t *testing.T) {
	got := swaggerPathToSampleName("get", "/users/{id}")
	want := "GET__users_{id}.json"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFindRoute(t *testing.T) {
	routes := []Route{
		{Method: "GET", Swagger: "/users/{id}", Regex: swaggerPathToRegex("/users/{id}"), SampleFile: "GET__users_{id}.json"},
		{Method: "POST", Swagger: "/users", Regex: swaggerPathToRegex("/users"), SampleFile: "POST__users.json"},
	}

	r := FindRoute(routes, "get", "/users/55")
	if r == nil || r.Swagger != "/users/{id}" {
		t.Fatalf("expected to find /users/{id}, got %#v", r)
	}

	if FindRoute(routes, "GET", "/nope") != nil {
		t.Fatalf("expected nil")
	}
}

func TestBuildRoutes(t *testing.T) {
	doc := &openapi3.T{
		Paths: &openapi3.Paths{},
	}

	paths := openapi3.NewPaths()
	paths.Set("/users/{id}", &openapi3.PathItem{
		Get: &openapi3.Operation{Responses: openapi3.NewResponses()},
	})
	paths.Set("/users", &openapi3.PathItem{
		Post: &openapi3.Operation{Responses: openapi3.NewResponses()},
	})
	doc.Paths = paths

	spec := &Spec{Doc3: doc}
	routes := BuildRoutes(spec)

	var foundGet, foundPost bool
	for _, r := range routes {
		if r.Method == "GET" && r.Swagger == "/users/{id}" {
			foundGet = true
			if r.SampleFile != "GET__users_{id}.json" {
				t.Fatalf("bad sample file: %q", r.SampleFile)
			}
			if !r.Regex.MatchString("/users/1") {
				t.Fatalf("regex should match")
			}
		}
		if r.Method == "POST" && r.Swagger == "/users" {
			foundPost = true
		}
	}

	if !foundGet || !foundPost {
		t.Fatalf("missing routes: foundGet=%v foundPost=%v routes=%#v", foundGet, foundPost, routes)
	}
}

func TestBuildRoutes_NilGuards(t *testing.T) {
	if got := BuildRoutes(nil); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}

	if got := BuildRoutes(&Spec{Doc3: nil}); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}

	if got := BuildRoutes(&Spec{Doc3: &openapi3.T{}}); got != nil && len(got) != 0 {
		t.Fatalf("expected nil or empty, got %#v", got)
	}
}
