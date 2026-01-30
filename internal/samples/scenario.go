package samples

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ozgen/openapi-sample-emulator/logger"
	"github.com/sirupsen/logrus"
)

type ScenarioResolver interface {
	ResolveScenarioFile(
		sc *Scenario,
		method string,
		swaggerTpl string,
		actualPath string,
	) (file string, state string, err error)
}

type Scenario struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"` // "step" | "time"

	Key struct {
		PathParam string `json:"pathParam"`
	} `json:"key"`

	// step mode
	Sequence []ScenarioEntry `json:"sequence,omitempty"`

	// time mode
	Timeline []TimelineEntry `json:"timeline,omitempty"`

	Behavior Behavior `json:"behavior"`
}

type ScenarioEntry struct {
	State string `json:"state"`
	File  string `json:"file"`
}

type TimelineEntry struct {
	AfterSec int64  `json:"afterSec"`
	State    string `json:"state"`
	File     string `json:"file"`
}

type Behavior struct {
	AdvanceOn  []MatchRule `json:"advanceOn,omitempty"`
	ResetOn    []MatchRule `json:"resetOn,omitempty"`
	StartOn    []MatchRule `json:"startOn,omitempty"`
	RepeatLast bool        `json:"repeatLast"`
	Loop       bool        `json:"loop,omitempty"`
}

type MatchRule struct {
	Method string `json:"method"`
	Path   string `json:"path,omitempty"`
}

// ScenarioEngine holds runtime state (in-memory).
type ScenarioEngine struct {
	mu        sync.Mutex
	stepIndex map[string]int
	startedAt map[string]time.Time
	log       *logrus.Logger
}

func NewScenarioEngine() *ScenarioEngine {
	return &ScenarioEngine{
		stepIndex: map[string]int{},
		startedAt: map[string]time.Time{},
		log:       logger.GetLogger(),
	}
}

func LoadScenario(scenarioPath string) (*Scenario, error) {
	log := logger.GetLogger()

	b, err := os.ReadFile(scenarioPath)
	if err != nil {
		return nil, err
	}

	var sc Scenario
	if err := json.Unmarshal(b, &sc); err != nil {
		log.WithError(err).Error("failed to parse scenario.json")
		return nil, fmt.Errorf("parse scenario.json: %w", err)
	}

	if sc.Version != 1 {
		log.WithField("version", sc.Version).Error("unsupported scenario version")
		return nil, fmt.Errorf("unsupported scenario version: %d", sc.Version)
	}

	sc.Mode = strings.TrimSpace(sc.Mode)
	if sc.Mode != "step" && sc.Mode != "time" {
		log.WithField("mode", sc.Mode).Error("invalid scenario mode")
		return nil, fmt.Errorf("invalid scenario mode: %q", sc.Mode)
	}

	if strings.TrimSpace(sc.Key.PathParam) == "" {
		log.Error("scenario.key.pathParam is required")
		return nil, fmt.Errorf("scenario.key.pathParam is required")
	}

	// validate mode-specific requirements
	switch sc.Mode {
	case "step":
		if len(sc.Sequence) == 0 {
			log.Error("scenario.sequence is required")
			return nil, fmt.Errorf("step mode requires non-empty sequence")
		}
	case "time":
		if len(sc.Timeline) == 0 {
			log.Error("scenario.timeline is required")
			return nil, fmt.Errorf("time mode requires non-empty timeline")
		}
		// ensure sorted
		for i := 1; i < len(sc.Timeline); i++ {
			if sc.Timeline[i].AfterSec < sc.Timeline[i-1].AfterSec {
				return nil, fmt.Errorf("timeline must be sorted by afterMs ascending")
			}
		}
	}

	return &sc, nil
}

func (e *ScenarioEngine) ResolveScenarioFile(
	sc *Scenario,
	method string,
	swaggerTpl string,
	actualPath string,
) (file string, state string, err error) {
	method = strings.ToUpper(method)

	keyVal, ok := extractPathParam(swaggerTpl, actualPath, sc.Key.PathParam)
	if !ok || strings.TrimSpace(keyVal) == "" {
		e.log.WithFields(logrus.Fields{
			"swaggerTpl": swaggerTpl,
			"actualPath": actualPath,
			"want":       sc.Key.PathParam,
		}).Error("failed to extract key path param")
		return "", "", fmt.Errorf(
			"cannot extract key path param %q from path %q using template %q",
			sc.Key.PathParam, actualPath, swaggerTpl,
		)
	}

	k := scenarioRuntimeKey(swaggerTpl, keyVal)

	if matchesAny(sc.Behavior.ResetOn, method, actualPath) {
		e.mu.Lock()
		delete(e.stepIndex, k)
		delete(e.startedAt, k)
		e.mu.Unlock()

		e.log.WithFields(logrus.Fields{
			"key":        k,
			"actualPath": actualPath,
			"method":     method,
		}).Debug("scenario state reset")
	}

	switch sc.Mode {
	case "step":
		return e.resolveStep(k, sc, method)
	case "time":
		return e.resolveTime(k, sc, method, actualPath)
	default:
		return "", "", fmt.Errorf("unsupported mode %q", sc.Mode)
	}
}

func (e *ScenarioEngine) resolveStep(k string, sc *Scenario, method string) (string, string, error) {
	if len(sc.Sequence) == 0 {
		return "", "", fmt.Errorf("step mode requires non-empty sequence")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	idx := e.stepIndex[k]
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sc.Sequence) {
		idx = len(sc.Sequence) - 1
	}

	entry := sc.Sequence[idx]

	if matchesAny(sc.Behavior.AdvanceOn, method, "") {
		next := idx + 1

		if next >= len(sc.Sequence) {
			switch {
			case sc.Behavior.Loop:
				next = 0
			case sc.Behavior.RepeatLast:
				next = len(sc.Sequence) - 1
			default:
				next = len(sc.Sequence) - 1
			}
		}

		e.stepIndex[k] = next
	} else {
		e.stepIndex[k] = idx
	}

	return entry.File, entry.State, nil
}

func (e *ScenarioEngine) resolveTime(k string, sc *Scenario, method string, actualPath string) (string, string, error) {
	if len(sc.Timeline) == 0 {
		return "", "", fmt.Errorf("time mode requires non-empty timeline")
	}

	e.mu.Lock()
	t0, ok := e.startedAt[k]
	if !ok {
		if len(sc.Behavior.StartOn) == 0 || matchesAny(sc.Behavior.StartOn, method, actualPath) {
			t0 = time.Now()
			e.startedAt[k] = t0
		} else {
			t0 = time.Now()
			e.startedAt[k] = t0
		}
	}
	elapsedSec := int64(time.Since(t0).Seconds())
	e.mu.Unlock()

	total := sc.Timeline[len(sc.Timeline)-1].AfterSec
	if total < 0 {
		total = 0
	}

	if sc.Behavior.Loop && total > 0 {
		elapsedSec = elapsedSec % (total + 1)
	} else if sc.Behavior.RepeatLast && elapsedSec > total {
		elapsedSec = total
	} else if elapsedSec > total {
		elapsedSec = total
	}

	chosen := sc.Timeline[0]
	for _, t := range sc.Timeline {
		if t.AfterSec <= elapsedSec {
			chosen = t
		} else {
			break
		}
	}

	return chosen.File, chosen.State, nil
}

func scenarioRuntimeKey(swaggerTpl, keyVal string) string {
	return strings.ToUpper(strings.TrimSpace(swaggerTpl)) + "::" + keyVal
}

func matchesAny(rules []MatchRule, method string, actualPath string) bool {
	method = strings.ToUpper(method)
	for _, r := range rules {
		if strings.ToUpper(strings.TrimSpace(r.Method)) != method {
			continue
		}
		p := strings.TrimSpace(r.Path)
		if p == "" {
			return true
		}
		if matchTemplatePath(p, actualPath) {
			return true
		}
	}
	return false
}

// matchTemplatePath supports "/scans/{id}" matching "/scans/123"
func matchTemplatePath(tpl, actual string) bool {
	tplParts := strings.Split(strings.Trim(tpl, "/"), "/")
	actParts := strings.Split(strings.Trim(actual, "/"), "/")
	if len(tplParts) != len(actParts) {
		return false
	}
	for i := range tplParts {
		t := tplParts[i]
		a := actParts[i]
		if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
			continue
		}
		if t != a {
			return false
		}
	}
	return true
}

func extractPathParam(swaggerTpl, actualPath, want string) (string, bool) {
	tplParts := strings.Split(strings.Trim(swaggerTpl, "/"), "/")
	actParts := strings.Split(strings.Trim(actualPath, "/"), "/")
	if len(tplParts) != len(actParts) {
		return "", false
	}
	for i := range tplParts {
		p := tplParts[i]
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(p, "{"), "}")
			if name == want {
				return actParts[i], true
			}
		}
	}
	return "", false
}

func ScenarioPathForSwagger(baseDir, swaggerPath, filename string) string {
	pathDir := strings.TrimPrefix(swaggerPath, "/")
	pathDir = filepath.FromSlash(pathDir)
	return filepath.Join(baseDir, pathDir, filename)
}
