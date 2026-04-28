package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv            string
	HTTPAddr          string
	DatabaseURL       string
	MigrationsDir     string
	LogLevel          string
	SessionSecret     string
	CookieSecure      bool
	SessionTTL        time.Duration
	APIKeyPepper      string
	APIKeyPrefix      string
	AdminSeedEmail    string
	AdminSeedPassword string
	ReadinessTimeout  time.Duration
	ShutdownTimeout   time.Duration
	OmniRoute         OmniRouteConfig
	UsageSyncWorker   UsageSyncWorkerConfig
}

type OmniRouteConfig struct {
	Enabled         bool
	BaseURL         string
	ManagementToken string
	UsageSyncMode   string
	HTTPTimeout     time.Duration
	CallLogLimit    int
}

type UsageSyncWorkerConfig struct {
	Enabled    bool
	Interval   time.Duration
	LockKey    string
	BatchLimit int
	StartDelay time.Duration
}

func Load() (Config, error) {
	readinessTimeout, err := durationFromEnv("READINESS_TIMEOUT", 2*time.Second)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := durationFromEnv("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	sessionTTL, err := durationFromEnv("SESSION_TTL", 168*time.Hour)
	if err != nil {
		return Config{}, err
	}

	cookieSecure, err := boolFromEnv("COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}

	omniRouteEnabled, err := boolFromEnv("OMNIROUTE_ENABLED", false)
	if err != nil {
		return Config{}, err
	}

	omniRouteHTTPTimeoutSeconds, err := intFromEnv("OMNIROUTE_HTTP_TIMEOUT_SECONDS", 15)
	if err != nil {
		return Config{}, err
	}
	omniRouteCallLogLimit, err := intFromEnv("OMNIROUTE_CALL_LOG_LIMIT", 100)
	if err != nil {
		return Config{}, err
	}
	usageSyncWorkerEnabled, err := boolFromEnv("USAGE_SYNC_WORKER_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	usageSyncIntervalSeconds, err := intFromEnv("USAGE_SYNC_INTERVAL_SECONDS", 60)
	if err != nil {
		return Config{}, err
	}
	usageSyncBatchLimit, err := intFromEnv("USAGE_SYNC_BATCH_LIMIT", 0)
	if err != nil {
		return Config{}, err
	}
	usageSyncStartDelaySeconds, err := intFromEnv("USAGE_SYNC_START_DELAY_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:            stringFromEnv("APP_ENV", "development"),
		HTTPAddr:          stringFromEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:       stringFromEnv("DATABASE_URL", "postgres://slai:slai@localhost:5432/slai?sslmode=disable"),
		MigrationsDir:     stringFromEnv("MIGRATIONS_DIR", "../../db/migrations"),
		LogLevel:          stringFromEnv("LOG_LEVEL", "info"),
		SessionSecret:     stringFromEnv("SESSION_SECRET", "dev-only-change-me"),
		CookieSecure:      cookieSecure,
		SessionTTL:        sessionTTL,
		APIKeyPepper:      stringFromEnv("API_KEY_PEPPER", "dev-only-change-me-api-key-pepper"),
		APIKeyPrefix:      stringFromEnv("API_KEY_PREFIX", "sk_slai"),
		AdminSeedEmail:    stringFromEnv("ADMIN_SEED_EMAIL", ""),
		AdminSeedPassword: stringFromEnv("ADMIN_SEED_PASSWORD", ""),
		ReadinessTimeout:  readinessTimeout,
		ShutdownTimeout:   shutdownTimeout,
		OmniRoute: OmniRouteConfig{
			Enabled:         omniRouteEnabled,
			BaseURL:         stringFromEnv("OMNIROUTE_BASE_URL", "http://localhost:4000"),
			ManagementToken: stringFromEnv("OMNIROUTE_MANAGEMENT_TOKEN", ""),
			UsageSyncMode:   stringFromEnv("OMNIROUTE_USAGE_SYNC_MODE", "call_logs"),
			HTTPTimeout:     time.Duration(omniRouteHTTPTimeoutSeconds) * time.Second,
			CallLogLimit:    omniRouteCallLogLimit,
		},
		UsageSyncWorker: UsageSyncWorkerConfig{
			Enabled:    usageSyncWorkerEnabled,
			Interval:   time.Duration(usageSyncIntervalSeconds) * time.Second,
			LockKey:    stringFromEnv("USAGE_SYNC_LOCK_KEY", "slai_usage_sync"),
			BatchLimit: usageSyncBatchLimit,
			StartDelay: time.Duration(usageSyncStartDelaySeconds) * time.Second,
		},
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.OmniRoute.Enabled {
		if cfg.OmniRoute.BaseURL == "" {
			return Config{}, fmt.Errorf("OMNIROUTE_BASE_URL is required when OMNIROUTE_ENABLED=true")
		}
		if cfg.OmniRoute.ManagementToken == "" {
			return Config{}, fmt.Errorf("OMNIROUTE_MANAGEMENT_TOKEN is required when OMNIROUTE_ENABLED=true")
		}
	}
	if cfg.OmniRoute.CallLogLimit <= 0 {
		cfg.OmniRoute.CallLogLimit = 100
	}
	if cfg.UsageSyncWorker.Interval <= 0 {
		cfg.UsageSyncWorker.Interval = 60 * time.Second
	}
	if cfg.UsageSyncWorker.StartDelay < 0 {
		cfg.UsageSyncWorker.StartDelay = 10 * time.Second
	}
	if cfg.UsageSyncWorker.LockKey == "" {
		cfg.UsageSyncWorker.LockKey = "slai_usage_sync"
	}
	if cfg.UsageSyncWorker.BatchLimit <= 0 {
		cfg.UsageSyncWorker.BatchLimit = cfg.OmniRoute.CallLogLimit
	}

	return cfg, nil
}

func stringFromEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func intFromEnv(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s as int: %w", key, err)
	}
	return parsed, nil
}

func boolFromEnv(key string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s as bool: %w", key, err)
	}
	return parsed, nil
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s as duration: %w", key, err)
	}
	return parsed, nil
}
