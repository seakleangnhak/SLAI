package config_test

import (
	"testing"
	"time"

	"github.com/slai/slai/services/api/internal/config"
)

func TestUsageSyncWorkerConfigDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("OMNIROUTE_CALL_LOG_LIMIT", "250")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UsageSyncWorker.Enabled {
		t.Fatal("worker should be disabled by default")
	}
	if cfg.UsageSyncWorker.Interval != 60*time.Second {
		t.Fatalf("interval = %s, want 60s", cfg.UsageSyncWorker.Interval)
	}
	if cfg.UsageSyncWorker.StartDelay != 10*time.Second {
		t.Fatalf("start delay = %s, want 10s", cfg.UsageSyncWorker.StartDelay)
	}
	if cfg.UsageSyncWorker.LockKey != "slai_usage_sync" {
		t.Fatalf("lock key = %q", cfg.UsageSyncWorker.LockKey)
	}
	if cfg.UsageSyncWorker.BatchLimit != 250 {
		t.Fatalf("batch limit = %d, want 250", cfg.UsageSyncWorker.BatchLimit)
	}
}

func TestUsageSyncWorkerConfigExplicitValues(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("USAGE_SYNC_WORKER_ENABLED", "true")
	t.Setenv("USAGE_SYNC_INTERVAL_SECONDS", "17")
	t.Setenv("USAGE_SYNC_LOCK_KEY", "custom_usage_sync")
	t.Setenv("USAGE_SYNC_BATCH_LIMIT", "33")
	t.Setenv("USAGE_SYNC_START_DELAY_SECONDS", "2")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UsageSyncWorker.Enabled {
		t.Fatal("worker should be enabled")
	}
	if cfg.UsageSyncWorker.Interval != 17*time.Second {
		t.Fatalf("interval = %s, want 17s", cfg.UsageSyncWorker.Interval)
	}
	if cfg.UsageSyncWorker.StartDelay != 2*time.Second {
		t.Fatalf("start delay = %s, want 2s", cfg.UsageSyncWorker.StartDelay)
	}
	if cfg.UsageSyncWorker.LockKey != "custom_usage_sync" {
		t.Fatalf("lock key = %q", cfg.UsageSyncWorker.LockKey)
	}
	if cfg.UsageSyncWorker.BatchLimit != 33 {
		t.Fatalf("batch limit = %d, want 33", cfg.UsageSyncWorker.BatchLimit)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"READINESS_TIMEOUT",
		"SHUTDOWN_TIMEOUT",
		"SESSION_TTL",
		"COOKIE_SECURE",
		"OMNIROUTE_ENABLED",
		"OMNIROUTE_BASE_URL",
		"OMNIROUTE_MANAGEMENT_TOKEN",
		"OMNIROUTE_HTTP_TIMEOUT_SECONDS",
		"OMNIROUTE_CALL_LOG_LIMIT",
		"USAGE_SYNC_WORKER_ENABLED",
		"USAGE_SYNC_INTERVAL_SECONDS",
		"USAGE_SYNC_LOCK_KEY",
		"USAGE_SYNC_BATCH_LIMIT",
		"USAGE_SYNC_START_DELAY_SECONDS",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL", "postgres://slai:slai@localhost:5432/slai?sslmode=disable")
}
