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
}

type OmniRouteConfig struct {
	Enabled         bool
	BaseURL         string
	ManagementToken string
	UsageSyncMode   string
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
		},
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func stringFromEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
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
