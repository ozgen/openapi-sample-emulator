package server

import (
	"reflect"
	"testing"
	"time"
)

func TestExpandFlowSpec(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "empty -> nil",
			in:   "",
			want: nil,
		},
		{
			name: "simple list",
			in:   "requested,running,succeeded",
			want: []string{"requested", "running", "succeeded"},
		},
		{
			name: "trimming and empty parts",
			in:   "  requested ,  , running  ,succeeded  ",
			want: []string{"requested", "running", "succeeded"},
		},
		{
			name: "star expansion",
			in:   "requested,running*3,succeeded",
			want: []string{"requested", "running.1", "running.2", "running.3", "succeeded"},
		},
		{
			name: "invalid star count keeps literal token",
			in:   "running*0,done",
			want: []string{"running*0", "done"},
		},
		{
			name: "invalid star non-numeric keeps literal token",
			in:   "running*abc,done",
			want: []string{"running*abc", "done"},
		},
		{
			name: "star with spaces",
			in:   "running * 2",
			want: []string{"running.1", "running.2"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := expandFlowSpec(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expandFlowSpec(%q)\nwant: %#v\ngot:  %#v", tt.in, tt.want, got)
			}
		})
	}
}

func TestNewStateFlow_Enabled(t *testing.T) {
	sf := NewStateFlow(StateFlowConfig{
		FlowSpec:        "",
		DefaultStepSecs: 2,
	})
	if sf.Enabled() {
		t.Fatalf("expected disabled when FlowSpec empty")
	}

	sf = NewStateFlow(StateFlowConfig{
		FlowSpec:        "a,b",
		DefaultStepSecs: 2,
	})
	if !sf.Enabled() {
		t.Fatalf("expected enabled when FlowSpec non-empty")
	}
}

func TestStateFlow_Current_CountMode_ProgressesByCalls(t *testing.T) {
	sf := NewStateFlow(StateFlowConfig{
		FlowSpec:        "requested,running*2,succeeded",
		StepCalls:       2, // advance every 2 calls
		DefaultStepSecs: 2,
		ResetOnLast:     false,
	})

	key := "GET /items/{id} :: 123"

	// states expanded: requested, running.1, running.2, succeeded
	// stepCalls=2 => each state lasts 2 calls
	got := []string{
		sf.Current(key), // requested
		sf.Current(key), // requested
		sf.Current(key), // running.1
		sf.Current(key), // running.1
		sf.Current(key), // running.2
		sf.Current(key), // running.2
		sf.Current(key), // succeeded
		sf.Current(key), // succeeded (clamped)
	}

	want := []string{
		"requested",
		"requested",
		"running.1",
		"running.1",
		"running.2",
		"running.2",
		"succeeded",
		"succeeded",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("count-mode progression mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestStateFlow_Current_CountMode_ResetOnLast(t *testing.T) {
	sf := NewStateFlow(StateFlowConfig{
		FlowSpec:        "requested,running,succeeded",
		StepCalls:       1,
		DefaultStepSecs: 2,
		ResetOnLast:     true,
	})

	key := "K"

	if st := sf.Current(key); st != "requested" {
		t.Fatalf("1st call: expected requested, got %q", st)
	}
	if st := sf.Current(key); st != "running" {
		t.Fatalf("2nd call: expected running, got %q", st)
	}
	if st := sf.Current(key); st != "succeeded" {
		t.Fatalf("3rd call: expected succeeded, got %q", st)
	}

	// After reset, it should start again
	if st := sf.Current(key); st != "requested" {
		t.Fatalf("after reset: expected requested, got %q", st)
	}
}

func TestStateFlow_Current_TimeMode_ProgressesByElapsed(t *testing.T) {
	sf := NewStateFlow(StateFlowConfig{
		FlowSpec:        "requested,running,succeeded",
		StepCalls:       0, // time mode
		StepSeconds:     1,
		DefaultStepSecs: 1,
		ResetOnLast:     false,
	})

	key := "K"

	// Make it deterministic by setting startedAt in the past.
	// step=1s:
	// elapsed 0.2s => idx 0 => requested
	// elapsed 1.2s => idx 1 => running
	// elapsed 2.2s => idx 2 => succeeded
	now := time.Now()

	sf.mu.Lock()
	sf.startedAt[key] = now.Add(-200 * time.Millisecond)
	sf.mu.Unlock()
	if st := sf.Current(key); st != "requested" {
		t.Fatalf("expected requested, got %q", st)
	}

	sf.mu.Lock()
	sf.startedAt[key] = now.Add(-1200 * time.Millisecond)
	sf.mu.Unlock()
	if st := sf.Current(key); st != "running" {
		t.Fatalf("expected running, got %q", st)
	}

	sf.mu.Lock()
	sf.startedAt[key] = now.Add(-2200 * time.Millisecond)
	sf.mu.Unlock()
	if st := sf.Current(key); st != "succeeded" {
		t.Fatalf("expected succeeded, got %q", st)
	}

	// Clamp beyond end
	sf.mu.Lock()
	sf.startedAt[key] = now.Add(-10 * time.Second)
	sf.mu.Unlock()
	if st := sf.Current(key); st != "succeeded" {
		t.Fatalf("expected clamp to succeeded, got %q", st)
	}
}

func TestStateFlow_Current_TimeMode_ResetOnLast(t *testing.T) {
	sf := NewStateFlow(StateFlowConfig{
		FlowSpec:        "requested,running,succeeded",
		StepCalls:       0,
		StepSeconds:     1,
		DefaultStepSecs: 1,
		ResetOnLast:     true,
	})

	key := "K"
	now := time.Now()

	sf.mu.Lock()
	sf.startedAt[key] = now.Add(-3 * time.Second)
	sf.mu.Unlock()

	if st := sf.Current(key); st != "succeeded" {
		t.Fatalf("expected succeeded, got %q", st)
	}

	if st := sf.Current(key); st != "requested" {
		t.Fatalf("after reset: expected requested, got %q", st)
	}
}

func TestExtractPathParam(t *testing.T) {
	t.Run("extracts id", func(t *testing.T) {
		got, ok := extractPathParam("/items/{id}", "/items/123", "id")
		if !ok {
			t.Fatalf("expected ok=true")
		}
		if got != "123" {
			t.Fatalf("value mismatch: want %q got %q", "123", got)
		}
	})

	t.Run("extracts nested sid", func(t *testing.T) {
		tpl := "/api/v1/items/{id}/sub/{sid}"
		act := "/api/v1/items/123/sub/999"

		got, ok := extractPathParam(tpl, act, "sid")
		if !ok {
			t.Fatalf("expected ok=true")
		}
		if got != "999" {
			t.Fatalf("value mismatch: want %q got %q", "999", got)
		}
	})

	t.Run("extracts nested id", func(t *testing.T) {
		tpl := "/api/v1/items/{id}/sub/{sid}"
		act := "/api/v1/items/123/sub/999"

		got, ok := extractPathParam(tpl, act, "id")
		if !ok {
			t.Fatalf("expected ok=true")
		}
		if got != "123" {
			t.Fatalf("value mismatch: want %q got %q", "123", got)
		}
	})

	t.Run("mismatch length -> false", func(t *testing.T) {
		got, ok := extractPathParam("/items/{id}", "/items/123/extra", "id")
		if ok {
			t.Fatalf("expected ok=false, got ok=true with %q", got)
		}
	})

	t.Run("param not present -> false", func(t *testing.T) {
		got, ok := extractPathParam("/items/{id}", "/items/123", "scan_id")
		if ok {
			t.Fatalf("expected ok=false, got ok=true with %q", got)
		}
	})
}

func TestMakeStateKey(t *testing.T) {
	t.Run("includes id when extractable", func(t *testing.T) {
		got := makeStateKey("get", "/items/{id}", "/items/abc", "id")
		want := "GET /items/{id} :: abc"
		if got != want {
			t.Fatalf("want %q got %q", want, got)
		}
	})

	t.Run("no id param configured", func(t *testing.T) {
		got := makeStateKey("POST", "/items/{id}", "/items/abc", "")
		want := "POST /items/{id}"
		if got != want {
			t.Fatalf("want %q got %q", want, got)
		}
	})

	t.Run("id param configured but cannot extract -> base only", func(t *testing.T) {
		got := makeStateKey("GET", "/items/{id}", "/items/abc/extra", "id")
		want := "GET /items/{id}"
		if got != want {
			t.Fatalf("want %q got %q", want, got)
		}
	})

	t.Run("id extracted but empty -> base only", func(t *testing.T) {
		got := makeStateKey("GET", "/items/{id}", "/items/", "id")
		want := "GET /items/{id}"
		if got != want {
			t.Fatalf("want %q got %q", want, got)
		}
	})
}
