package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// sentinel handler that always returns 200.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// ── RequireAPIKey ─────────────────────────────────────────────────────────────

func TestRequireAPIKey_ValidKey(t *testing.T) {
	h := RequireAPIKey("secret")(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", "secret")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequireAPIKey_MissingKey(t *testing.T) {
	h := RequireAPIKey("secret")(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAPIKey_WrongKey(t *testing.T) {
	h := RequireAPIKey("secret")(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", "wrong")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// Ensure the key comparison does not short-circuit on length mismatch
// (constant-time guarantee is structural; we verify correct rejection).
func TestRequireAPIKey_PrefixAttack(t *testing.T) {
	h := RequireAPIKey("secret")(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", "secre") // one byte short
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// ── RateLimit ─────────────────────────────────────────────────────────────────

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	// burst=5 means first 5 requests from the same IP are allowed.
	h := RateLimit(100, 5)(okHandler)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:9999"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}
}

func TestRateLimit_BlocksOverLimit(t *testing.T) {
	// burst=2; third request from the same IP should be rejected.
	h := RateLimit(1, 2)(okHandler)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "5.6.7.8:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "5.6.7.8:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over-limit request: status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}

	if rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, want %q", rec.Header().Get("Retry-After"), "1")
	}
}

func TestRateLimit_DifferentIPsAreIndependent(t *testing.T) {
	// Two IPs each exhausting burst=1 should not affect each other.
	h := RateLimit(1, 1)(okHandler)

	for _, ip := range []string{"10.0.0.1:1", "10.0.0.2:1"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("ip %s: status = %d, want %d", ip, rec.Code, http.StatusOK)
		}
	}
}

// ── Chain ─────────────────────────────────────────────────────────────────────

func TestChain_ExecutesOuterFirst(t *testing.T) {
	order := []string{}

	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1")
			next.ServeHTTP(w, r)
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2")
			next.ServeHTTP(w, r)
		})
	}

	h := Chain(okHandler, m1, m2)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if len(order) != 2 || order[0] != "m1" || order[1] != "m2" {
		t.Fatalf("execution order = %v, want [m1 m2]", order)
	}
}
