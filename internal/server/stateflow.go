package server

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type StateFlow struct {
	mu sync.Mutex

	states []string

	// time mode
	startedAt map[string]time.Time
	step      time.Duration

	// count mode (if stepCalls > 0)
	stepCalls int
	callCount map[string]int

	resetOnLast bool
}

type StateFlowConfig struct {
	FlowSpec        string // "requested,running*4,succeeded"
	StepSeconds     int    // used if StepCalls == 0
	StepCalls       int    // if > 0, count-based progression
	DefaultStepSecs int    // fallback if StepSeconds <= 0
	ResetOnLast     bool
}

func NewStateFlow(cfg StateFlowConfig) *StateFlow {
	states := expandFlowSpec(cfg.FlowSpec)
	if len(states) == 0 {
		// If no flow is configured, we keep it empty: caller can treat as disabled.
		states = nil
	}

	stepSecs := cfg.StepSeconds
	if stepSecs <= 0 {
		if cfg.DefaultStepSecs > 0 {
			stepSecs = cfg.DefaultStepSecs
		} else {
			stepSecs = 2
		}
	}

	sf := &StateFlow{
		states:      states,
		startedAt:   map[string]time.Time{},
		step:        time.Duration(stepSecs) * time.Second,
		stepCalls:   cfg.StepCalls,
		callCount:   map[string]int{},
		resetOnLast: cfg.ResetOnLast,
	}
	return sf
}

// Enabled tells if the state flow should be used.
func (sf *StateFlow) Enabled() bool {
	return len(sf.states) > 0
}

// Current returns the current state for a given key.
// - If count mode is enabled (stepCalls > 0): state advances every N calls.
// - Otherwise: state advances every step duration.
func (sf *StateFlow) Current(key string) string {
	sf.mu.Lock()
	defer sf.mu.Unlock()

	if len(sf.states) == 0 {
		return ""
	}

	// Count-based progression
	if sf.stepCalls > 0 {
		sf.callCount[key]++
		idx := (sf.callCount[key] - 1) / sf.stepCalls
		if idx < 0 {
			idx = 0
		}

		// If we reached/passed the last state
		if idx >= len(sf.states) {
			idx = len(sf.states) - 1
		}
		state := sf.states[idx]

		// Reset AFTER returning last state once
		if sf.resetOnLast && idx == len(sf.states)-1 {
			delete(sf.callCount, key)
			delete(sf.startedAt, key)
		}
		return state
	}

	// Time-based progression
	t0, ok := sf.startedAt[key]
	if !ok {
		t0 = time.Now()
		sf.startedAt[key] = t0
	}
	elapsed := time.Since(t0)
	idx := int(elapsed / sf.step)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sf.states) {
		idx = len(sf.states) - 1
	}
	state := sf.states[idx]

	// Reset AFTER returning last state (once time reaches end)
	if sf.resetOnLast && idx == len(sf.states)-1 {
		delete(sf.callCount, key)
		delete(sf.startedAt, key)
	}

	return state
}

func expandFlowSpec(spec string) []string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}

	var out []string
	parts := strings.Split(spec, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		name := p
		count := 1

		// "running*4"
		if i := strings.LastIndex(p, "*"); i > 0 && i < len(p)-1 {
			left := strings.TrimSpace(p[:i])
			right := strings.TrimSpace(p[i+1:])
			if left != "" {
				if n, err := strconv.Atoi(right); err == nil && n > 0 {
					name = left
					count = n
				}
			}
		}

		if count == 1 {
			out = append(out, name)
			continue
		}

		for i := 1; i <= count; i++ {
			out = append(out, fmt.Sprintf("%s.%d", name, i))
		}
	}

	return out
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

func makeStateKey(method, swaggerTpl, actualPath, idParam string) string {
	base := strings.ToUpper(method) + " " + swaggerTpl

	if idParam == "" {
		return base
	}
	if id, ok := extractPathParam(swaggerTpl, actualPath, idParam); ok && id != "" {
		return base + " :: " + id
	}
	return base
}
