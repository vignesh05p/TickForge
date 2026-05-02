package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vigneshprabhu/tickforge/internal/config"
)

func TestHealthz(t *testing.T) {
	srv := New(config.Config{}, nil)
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

func TestReadyz(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		srv := New(config.Config{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("not ready", func(t *testing.T) {
		srv := New(config.Config{}, func(context.Context) error {
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
