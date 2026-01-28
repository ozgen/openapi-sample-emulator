package sample

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Envelope struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
}

// Response is what the server will write to the client.
type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

// Load tries to load a sample file by name from dir.
func Load(dir, fileName string) (*Response, error) {
	p := filepath.Join(dir, fileName)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read sample %s: %w", p, err)
	}
	raw := strings.TrimSpace(string(b))

	// Try envelope parse
	var env Envelope
	if err := json.Unmarshal([]byte(raw), &env); err == nil &&
		(env.Status != 0 || env.Headers != nil || env.Body != nil) {
		if env.Status == 0 {
			env.Status = 200
		}
		if env.Headers == nil {
			env.Headers = map[string]string{"content-type": "application/json"}
		}
		bodyBytes, err := json.Marshal(env.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal envelope body: %w", err)
		}
		return &Response{
			Status:  env.Status,
			Headers: env.Headers,
			Body:    bodyBytes,
		}, nil
	}

	return &Response{
		Status:  200,
		Headers: map[string]string{"content-type": "application/json"},
		Body:    []byte(raw),
	}, nil
}
