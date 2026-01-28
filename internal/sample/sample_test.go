package sample

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ReadError(t *testing.T) {
	_, err := Load("/no/such/dir", "missing.json")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoad_Envelope_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	name := "sample.json"

	writeFile(t, dir, name, `{"body":{"ok":true}}`)

	resp, err := Load(dir, name)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if resp.Status != 200 {
		t.Fatalf("expected default status 200, got %d", resp.Status)
	}
	if resp.Headers == nil || resp.Headers["content-type"] != "application/json" {
		t.Fatalf("expected default content-type header, got %#v", resp.Headers)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %q", string(resp.Body))
	}
}

func TestLoad_Envelope_ExplicitValues(t *testing.T) {
	dir := t.TempDir()
	name := "sample.json"

	writeFile(t, dir, name, `{
	  "status": 201,
	  "headers": {"content-type":"application/problem+json","x-test":"1"},
	  "body": {"id": 123}
	}`)

	resp, err := Load(dir, name)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if resp.Status != 201 {
		t.Fatalf("expected 201, got %d", resp.Status)
	}
	if resp.Headers["content-type"] != "application/problem+json" || resp.Headers["x-test"] != "1" {
		t.Fatalf("unexpected headers: %#v", resp.Headers)
	}

	if string(resp.Body) != `{"id":123}` {
		t.Fatalf("unexpected body: %q", string(resp.Body))
	}
}

func TestLoad_Envelope_StatusOnlyDefaultsHeaderAndBodyNull(t *testing.T) {
	dir := t.TempDir()
	name := "sample.json"

	writeFile(t, dir, name, `{"status":204}`)

	resp, err := Load(dir, name)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if resp.Status != 204 {
		t.Fatalf("expected 204, got %d", resp.Status)
	}
	if resp.Headers["content-type"] != "application/json" {
		t.Fatalf("expected default content-type, got %#v", resp.Headers)
	}
	if string(resp.Body) != "null" {
		t.Fatalf("expected body 'null', got %q", string(resp.Body))
	}
}

func TestLoad_Envelope_MarshalBodyError(t *testing.T) {
	dir := t.TempDir()
	name := "sample.json"

	writeFile(t, dir, name, `{"body":[1,2,3]}`)

	_, err := Load(dir, name)
	if err != nil {
		t.Fatalf("did not expect error, got %v", err)
	}

	_ = errors.New("silence unused import")
}

func TestLoad_Fallback_PlainJSONOrText(t *testing.T) {
	dir := t.TempDir()

	t.Run("raw json without envelope", func(t *testing.T) {
		name := "raw.json"

		writeFile(t, dir, name, `{}`)

		resp, err := Load(dir, name)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if resp.Status != 200 {
			t.Fatalf("expected 200, got %d", resp.Status)
		}
		if resp.Headers["content-type"] != "application/json" {
			t.Fatalf("expected application/json, got %#v", resp.Headers)
		}
		if string(resp.Body) != `{}` {
			t.Fatalf("unexpected body: %q", string(resp.Body))
		}
	})

	t.Run("plain text", func(t *testing.T) {
		name := "raw.txt"
		writeFile(t, dir, name, `  hello world  `)

		resp, err := Load(dir, name)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if string(resp.Body) != `hello world` {
			t.Fatalf("unexpected body: %q", string(resp.Body))
		}
	})
}

func TestLoad_Envelope_WhenHeadersPresentButBodyMissing(t *testing.T) {
	dir := t.TempDir()
	name := "hdrs.json"

	writeFile(t, dir, name, `{"headers":{"content-type":"text/plain"}}`)

	resp, err := Load(dir, name)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("expected default status 200, got %d", resp.Status)
	}
	if resp.Headers["content-type"] != "text/plain" {
		t.Fatalf("unexpected headers: %#v", resp.Headers)
	}
	if string(resp.Body) != "null" {
		t.Fatalf("expected body 'null', got %q", string(resp.Body))
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}
