package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vigneshprabhu/tickforge/internal/config"
)

// testConfig returns a minimal valid Config for use in server tests.
func testConfig() config.Config {
	return config.Config{
		APIKey:    "test-api-key",
		RateLimit: 1000,
		RateBurst: 1000,
	}
}

func TestHealthz(t *testing.T) {
	srv := New(testConfig(), nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestHealthz_NoAuthRequired(t *testing.T) {
	// /healthz must be reachable without X-API-Key.
	srv := New(testConfig(), nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	// Deliberately no X-API-Key header.
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (healthz should bypass auth)", rec.Code, http.StatusOK)
	}
}

func TestReadyz(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		srv := New(testConfig(), nil)
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("not ready", func(t *testing.T) {
		srv := New(testConfig(), func(context.Context) error {
			return errors.New("dependency down")
		})
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

func TestReadyz_NoAuthRequired(t *testing.T) {
	// /readyz must be reachable without X-API-Key.
	srv := New(testConfig(), nil)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (readyz should bypass auth)", rec.Code, http.StatusOK)
	}
}
