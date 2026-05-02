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
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("TICKFORGE_HTTP_ADDR", ":9090")
	t.Setenv("TICKFORGE_QUEUE_SIZE", "2048")
	t.Setenv("TICKFORGE_WORKERS", "8")
	t.Setenv("TICKFORGE_SHUTDOWN_TIMEOUT", "3s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != ":9090" || cfg.QueueSize != 2048 || cfg.WorkerCount != 8 || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("Load() = %+v", cfg)
	}
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	t.Setenv("TICKFORGE_QUEUE_SIZE", "0")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}
