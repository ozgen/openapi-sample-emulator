package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ozgen/openapi-sample-emulator/config"
	"github.com/ozgen/openapi-sample-emulator/internal/openapi"
	"github.com/ozgen/openapi-sample-emulator/internal/sample"
	"github.com/ozgen/openapi-sample-emulator/logger"
	"github.com/ozgen/openapi-sample-emulator/utils"
	"github.com/sirupsen/logrus"
)

type Config struct {
	Port           string
	SpecPath       string
	SamplesDir     string
	FallbackMode   config.FallbackMode
	ValidationMode config.ValidationMode
}

type Server struct {
	cfg    Config
	spec   *openapi.Spec
	routes []openapi.Route
	log    *logrus.Logger
}

func New(cfg Config) (*Server, error) {
	spec, err := openapi.LoadSpec(cfg.SpecPath)
	if err != nil {
		return nil, err
	}
	routes := openapi.BuildRoutes(spec)

	return &Server{
		cfg:    cfg,
		spec:   spec,
		routes: routes,
		log:    logger.GetLogger(),
	}, nil
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)

	addr := "0.0.0.0:" + s.cfg.Port

	s.log.Printf("mock listening on %s", addr)
	s.log.Printf(
		"spec=%s samples=%s fallback=%s",
		s.cfg.SpecPath,
		s.cfg.SamplesDir,
		s.cfg.FallbackMode,
	)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return server.ListenAndServe()
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	path := r.URL.Path

	if method == http.MethodGet && (path == "/health/alive" || path == "/health/ready" || path == "/health/started") {
		utils.WriteJSON(w, 200, map[string]any{"ok": true})
		return
	}

	rt := openapi.FindRoute(s.routes, method, path)
	if rt == nil {
		utils.WriteJSON(w, 404, map[string]any{
			"error":  "No route",
			"method": method,
			"path":   path,
		})
		return
	}

	if s.cfg.ValidationMode == config.ValidationRequired {
		if openapi.HasRequiredBodyParam(s.spec, rt.Swagger, rt.Method) {
			empty, err := openapi.IsEmptyBody(r)
			if err != nil {
				utils.WriteJSON(w, 400, map[string]any{"error": "Bad Request", "details": err.Error()})
				return
			}
			if empty {
				utils.WriteJSON(w, 400, map[string]any{
					"error":   "Bad Request",
					"details": "Request body is required by the API spec",
				})
				return
			}
		}
	}

	resp, err := sample.Load(s.cfg.SamplesDir, rt.SampleFile)
	if err != nil {
		if s.cfg.FallbackMode == config.FallbackOpenAPIExample {
			if body, ok := openapi.TryGetExampleBody(s.spec, rt.Swagger, rt.Method); ok {
				w.Header().Set("content-type", "application/json")
				w.WriteHeader(200)
				_, _ = w.Write(body)
				return
			}
		}

		utils.WriteJSON(w, 501, map[string]any{
			"error":              "No sample file for route",
			"expectedSampleFile": rt.SampleFile,
			"details":            err.Error(),
			"hint":               "Create the sample file, or set FALLBACK_MODE=openapi_examples and add response examples to swagger.json",
		})
		return
	}

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.Status)
	_, _ = w.Write(resp.Body)
}

func (s *Server) DebugRoutes() string {
	out := ""
	for _, r := range s.routes {
		out += fmt.Sprintf("%s %s -> %s\n", r.Method, r.Swagger, r.SampleFile)
	}
	return out
}
