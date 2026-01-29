package samples

import (
	"fmt"
	"github.com/ozgen/openapi-sample-emulator/config"
	"github.com/ozgen/openapi-sample-emulator/utils"
	"path/filepath"
	"strings"
)

type ResolverConfig struct {
	BaseDir string
	Layout  config.LayoutMode
	State   string
}

func ResolveSamplePath(cfg ResolverConfig, method, swaggerPath, legacyFlatFilename string) (string, error) {
	method = strings.ToUpper(method)
	relCandidates := buildCandidates(cfg, method, swaggerPath, legacyFlatFilename)
	if len(relCandidates) == 0 {
		return "", fmt.Errorf("no candidates for method=%s path=%s", method, swaggerPath)
	}

	for _, rel := range relCandidates {
		full := filepath.Join(cfg.BaseDir, rel)
		if utils.FileExists(full) {
			return full, nil
		}
	}

	return "", fmt.Errorf("no sample file found (tried: %v)", relCandidates)
}

func buildCandidates(cfg ResolverConfig, method, swaggerPath, legacyFlatFilename string) []string {
	layout := cfg.Layout
	if layout == "" {
		layout = config.LayoutAuto
	}

	var out []string

	if layout == config.LayoutAuto || layout == config.LayoutFolders {
		pathDir := strings.TrimPrefix(swaggerPath, "/")
		pathDir = filepath.FromSlash(pathDir)

		if st := strings.TrimSpace(cfg.State); st != "" {
			out = append(out, filepath.Join(pathDir, fmt.Sprintf("%s.%s.json", method, st)))
		}
		out = append(out, filepath.Join(pathDir, fmt.Sprintf("%s.json", method)))
	}

	// Flat legacy
	if layout == config.LayoutAuto || layout == config.LayoutFlat {
		out = append(out, legacyFlatFilename)
	}

	return out
}
