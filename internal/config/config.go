package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultHTTPAddr        = ":8080"
	defaultQueueSize       = 1024
	defaultWorkerCount     = 4
	defaultShutdownTimeout = 10 * time.Second
)

// Config contains process-level settings used to wire the service.
type Config struct {
	HTTPAddr        string
	QueueSize       int
	WorkerCount     int
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables and applies safe defaults.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        envString("TICKFORGE_HTTP_ADDR", defaultHTTPAddr),
		QueueSize:       defaultQueueSize,
		WorkerCount:     defaultWorkerCount,
		ShutdownTimeout: defaultShutdownTimeout,
	}

	var err error
	if cfg.QueueSize, err = envPositiveInt("TICKFORGE_QUEUE_SIZE", defaultQueueSize); err != nil {
		return Config{}, err
	}
	if cfg.WorkerCount, err = envPositiveInt("TICKFORGE_WORKERS", defaultWorkerCount); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = envPositiveDuration("TICKFORGE_SHUTDOWN_TIMEOUT", defaultShutdownTimeout); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envPositiveInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func envPositiveDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}
