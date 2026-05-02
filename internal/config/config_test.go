package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("TICKFORGE_HTTP_ADDR", "")
	t.Setenv("TICKFORGE_QUEUE_SIZE", "")
	t.Setenv("TICKFORGE_WORKERS", "")
	t.Setenv("TICKFORGE_SHUTDOWN_TIMEOUT", "")
	t.Setenv("TICKFORGE_API_KEY", "testkey")
	t.Setenv("TICKFORGE_RATE_LIMIT", "")
	t.Setenv("TICKFORGE_RATE_BURST", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != defaultHTTPAddr {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, defaultHTTPAddr)
	}
	if cfg.QueueSize != defaultQueueSize {
		t.Fatalf("QueueSize = %d, want %d", cfg.QueueSize, defaultQueueSize)
	}
	if cfg.WorkerCount != defaultWorkerCount {
		t.Fatalf("WorkerCount = %d, want %d", cfg.WorkerCount, defaultWorkerCount)
	}
	if cfg.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("ShutdownTimeout = %s, want %s", cfg.ShutdownTimeout, defaultShutdownTimeout)
	}
	if cfg.RateLimit != defaultRateLimit {
		t.Fatalf("RateLimit = %d, want %d", cfg.RateLimit, defaultRateLimit)
	}
	if cfg.RateBurst != defaultRateBurst {
		t.Fatalf("RateBurst = %d, want %d", cfg.RateBurst, defaultRateBurst)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("TICKFORGE_HTTP_ADDR", ":9090")
	t.Setenv("TICKFORGE_QUEUE_SIZE", "2048")
	t.Setenv("TICKFORGE_WORKERS", "8")
	t.Setenv("TICKFORGE_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("TICKFORGE_API_KEY", "supersecret")
	t.Setenv("TICKFORGE_RATE_LIMIT", "50")
	t.Setenv("TICKFORGE_RATE_BURST", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != ":9090" || cfg.QueueSize != 2048 || cfg.WorkerCount != 8 || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("Load() = %+v", cfg)
	}
	if cfg.APIKey != "supersecret" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "supersecret")
	}
	if cfg.RateLimit != 50 {
		t.Fatalf("RateLimit = %d, want 50", cfg.RateLimit)
	}
	if cfg.RateBurst != 10 {
		t.Fatalf("RateBurst = %d, want 10", cfg.RateBurst)
	}
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	t.Setenv("TICKFORGE_QUEUE_SIZE", "0")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadRejectsEmptyAPIKey(t *testing.T) {
	t.Setenv("TICKFORGE_API_KEY", "")
	t.Setenv("TICKFORGE_QUEUE_SIZE", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with empty API key: error = nil, want error")
	}
}

func TestLoadRejectsInvalidRateLimit(t *testing.T) {
	t.Setenv("TICKFORGE_API_KEY", "key")
	t.Setenv("TICKFORGE_RATE_LIMIT", "0")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with RATE_LIMIT=0: error = nil, want error")
	}
}
