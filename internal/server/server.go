package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/vigneshprabhu/tickforge/internal/config"
)

const contentTypeJSON = "application/json"

type ReadinessCheck func(context.Context) error

type Server struct {
	cfg        config.Config
	readyCheck ReadinessCheck
	mux        *http.ServeMux
	auth       func(http.Handler) http.Handler
	rateLimit  func(http.Handler) http.Handler
}

func New(cfg config.Config, readyCheck ReadinessCheck) *Server {
	if readyCheck == nil {
		readyCheck = func(context.Context) error { return nil }
	}

	s := &Server{
		cfg:        cfg,
		readyCheck: readyCheck,
		mux:        http.NewServeMux(),
		auth:       RequireAPIKey(cfg.APIKey),
		rateLimit:  RateLimit(cfg.RateLimit, cfg.RateBurst),
	}
	s.routes()
	return s
}

// Handler returns the root HTTP handler with the global rate-limiter applied.
// All requests pass through rate limiting; auth is applied per-route.
func (s *Server) Handler() http.Handler {
	return s.rateLimit(s.mux)
}

// HTTPServer returns a configured *http.Server ready to ListenAndServe.
func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// AddProtectedRoute registers pattern with auth + rate-limit middleware already
// applied. Phase 2 ingest and WebSocket handlers use this.
func (s *Server) AddProtectedRoute(pattern string, h http.Handler) {
	s.mux.Handle(pattern, s.auth(h))
}

// AddPublicRoute registers pattern without auth. Use for /metrics.
func (s *Server) AddPublicRoute(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

func (s *Server) routes() {
	// Health and readiness are public — load balancers and probes must reach
	// them without credentials.
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.readyCheck(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "NOT_READY", "service is not ready")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"details": map[string]any{},
		},
	})
}
