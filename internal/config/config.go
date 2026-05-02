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
	defaultRateLimit       = 100 // requests per second per IP
	defaultRateBurst       = 20  // token bucket burst size
)

// Config contains the process-level settings used to wire the service.
type Config struct {
	HTTPAddr        string
	QueueSize       int
	WorkerCount     int
	ShutdownTimeout time.Duration

	// APIKey is the static secret required on X-API-Key for protected endpoints.
	// The service refuses to start if this is empty.
	APIKey string

	// RateLimit is the sustained request rate (req/s) allowed per client IP
	// at the HTTP layer, before queue backpressure applies.
	RateLimit int

	// RateBurst is the token-bucket burst size for the per-IP rate limiter.
	RateBurst int
}

// Load reads configuration from environment variables and applies safe defaults.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        envString("TICKFORGE_HTTP_ADDR", defaultHTTPAddr),
		APIKey:          os.Getenv("TICKFORGE_API_KEY"),
		QueueSize:       defaultQueueSize,
		WorkerCount:     defaultWorkerCount,
		ShutdownTimeout: defaultShutdownTimeout,
		RateLimit:       defaultRateLimit,
		RateBurst:       defaultRateBurst,
	}

	if cfg.APIKey == "" {
		return Config{}, fmt.Errorf("TICKFORGE_API_KEY is required but not set")
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
	if cfg.RateLimit, err = envPositiveInt("TICKFORGE_RATE_LIMIT", defaultRateLimit); err != nil {
		return Config{}, err
	}
	if cfg.RateBurst, err = envPositiveInt("TICKFORGE_RATE_BURST", defaultRateBurst); err != nil {
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
