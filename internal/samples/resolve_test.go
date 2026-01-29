package samples

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ozgen/openapi-sample-emulator/config"
)

func TestBuildCandidates(t *testing.T) {
	tests := []struct {
		name   string
		cfg    ResolverConfig
		method string
		path   string
		flat   string
		want   []string
	}{
		{
			name:   "auto no state adds folder then flat",
			cfg:    ResolverConfig{Layout: config.LayoutAuto},
			method: "GET",
			path:   "/api/v1/items",
			flat:   "GET_api_v1_items.json",
			want: []string{
				filepath.Join(filepath.FromSlash("api/v1/items"), "GET.json"),
				"GET_api_v1_items.json",
			},
		},
		{
			name:   "auto with state prefers state then base then flat",
			cfg:    ResolverConfig{Layout: config.LayoutAuto, State: "running"},
			method: "GET",
			path:   "/api/v1/items",
			flat:   "GET_api_v1_items.json",
			want: []string{
				filepath.Join(filepath.FromSlash("api/v1/items"), "GET.running.json"),
				filepath.Join(filepath.FromSlash("api/v1/items"), "GET.json"),
				"GET_api_v1_items.json",
			},
		},
		{
			name:   "folders only no flat",
			cfg:    ResolverConfig{Layout: config.LayoutFolders},
			method: "POST",
			path:   "/api/v1/items",
			flat:   "POST_api_v1_items.json",
			want: []string{
				filepath.Join(filepath.FromSlash("api/v1/items"), "POST.json"),
			},
		},
		{
			name:   "flat only no folders",
			cfg:    ResolverConfig{Layout: config.LayoutFlat, State: "ignored"},
			method: "PUT",
			path:   "/api/v1/items/{id}",
			flat:   "PUT_api_v1_items_{id}.json",
			want: []string{
				"PUT_api_v1_items_{id}.json",
			},
		},
		{
			name:   "empty layout defaults to auto",
			cfg:    ResolverConfig{Layout: ""},
			method: "GET",
			path:   "/api/v1/items",
			flat:   "GET_api_v1_items.json",
			want: []string{
				filepath.Join(filepath.FromSlash("api/v1/items"), "GET.json"),
				"GET_api_v1_items.json",
			},
		},
		{
			name:   "state with whitespace is trimmed",
			cfg:    ResolverConfig{Layout: config.LayoutFolders, State: "  running  "},
			method: "GET",
			path:   "/api/v1/items",
			flat:   "GET_api_v1_items.json",
			want: []string{
				filepath.Join(filepath.FromSlash("api/v1/items"), "GET.running.json"),
				filepath.Join(filepath.FromSlash("api/v1/items"), "GET.json"),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := buildCandidates(tt.cfg, tt.method, tt.path, tt.flat)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildCandidates() mismatch\nwant: %#v\ngot:  %#v", tt.want, got)
			}
		})
	}
}

func TestResolveSamplePath_FindsStateSpecificFirst(t *testing.T) {
	baseDir := t.TempDir()
	cfg := ResolverConfig{
		BaseDir: baseDir,
		Layout:  config.LayoutFolders,
		State:   "running",
	}

	writeFileWithDirs(t, baseDir, filepath.Join("api", "v1", "items", "GET.running.json"), "x")
	writeFileWithDirs(t, baseDir, filepath.Join("api", "v1", "items", "GET.json"), "y")

	got, err := ResolveSamplePath(cfg, "get", "/api/v1/items", "GET_api_v1_items.json")
	if err != nil {
		t.Fatalf("ResolveSamplePath: %v", err)
	}

	want := filepath.Join(baseDir, "api", "v1", "items", "GET.running.json")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveSamplePath_Auto_PrefersFoldersOverFlat(t *testing.T) {
	baseDir := t.TempDir()
	cfg := ResolverConfig{
		BaseDir: baseDir,
		Layout:  config.LayoutAuto,
		State:   "",
	}

	writeFileWithDirs(t, baseDir, filepath.Join("api", "v1", "items", "GET.json"), "folder")
	writeFileWithDirs(t, baseDir, "GET_api_v1_items.json", "flat")

	got, err := ResolveSamplePath(cfg, "GET", "/api/v1/items", "GET_api_v1_items.json")
	if err != nil {
		t.Fatalf("ResolveSamplePath: %v", err)
	}

	want := filepath.Join(baseDir, "api", "v1", "items", "GET.json")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveSamplePath_FlatMode_FindsLegacy(t *testing.T) {
	baseDir := t.TempDir()
	cfg := ResolverConfig{
		BaseDir: baseDir,
		Layout:  config.LayoutFlat,
		State:   "running",
	}

	writeFileWithDirs(t, baseDir, "GET_api_v1_items.json", "flat")

	got, err := ResolveSamplePath(cfg, "GET", "/api/v1/items", "GET_api_v1_items.json")
	if err != nil {
		t.Fatalf("ResolveSamplePath: %v", err)
	}

	want := filepath.Join(baseDir, "GET_api_v1_items.json")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveSamplePath_UppercasesMethod(t *testing.T) {
	baseDir := t.TempDir()
	cfg := ResolverConfig{
		BaseDir: baseDir,
		Layout:  config.LayoutFolders,
		State:   "",
	}

	writeFileWithDirs(t, baseDir, filepath.Join("api", "v1", "items", "GET.json"), "x")

	got, err := ResolveSamplePath(cfg, "gEt", "/api/v1/items", "ignored.json")
	if err != nil {
		t.Fatalf("ResolveSamplePath: %v", err)
	}

	want := filepath.Join(baseDir, "api", "v1", "items", "GET.json")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveSamplePath_NoSampleFound_ReturnsError(t *testing.T) {
	baseDir := t.TempDir()
	cfg := ResolverConfig{
		BaseDir: baseDir,
		Layout:  config.LayoutAuto,
		State:   "running",
	}

	_, err := ResolveSamplePath(cfg, "GET", "/api/v1/items", "GET_api_v1_items.json")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestResolveSamplePath_NoCandidates_ReturnsError(t *testing.T) {
	baseDir := t.TempDir()
	cfg := ResolverConfig{
		BaseDir: baseDir,
		Layout:  config.LayoutFlat,
		State:   "",
	}

	_, err := ResolveSamplePath(cfg, "GET", "/api/v1/items", "")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func writeFileWithDirs(t *testing.T, baseDir, relPath, content string) {
	t.Helper()
	full := filepath.Join(baseDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}
