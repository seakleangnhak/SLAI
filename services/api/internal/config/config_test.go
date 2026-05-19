package config_test

import (
	"os"
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
	if cfg.Storage.Dir != "./storage" {
		t.Fatalf("storage dir = %q, want ./storage", cfg.Storage.Dir)
	}
	if cfg.Storage.PaymentProofMaxMB != 5 {
		t.Fatalf("proof max = %d, want 5", cfg.Storage.PaymentProofMaxMB)
	}
	if cfg.Storage.PaymentQRMaxMB != 2 {
		t.Fatalf("QR max = %d, want 2", cfg.Storage.PaymentQRMaxMB)
	}
	if cfg.SLAIPayment.Enabled {
		t.Fatal("SLAI payment should be disabled by default")
	}
	if cfg.SLAIPayment.BaseURL != "http://localhost:8090" {
		t.Fatalf("SLAI payment base URL = %q", cfg.SLAIPayment.BaseURL)
	}
	if cfg.SLAIPayment.DefaultExpiry != 30*time.Minute {
		t.Fatalf("SLAI payment expiry = %s, want 30m", cfg.SLAIPayment.DefaultExpiry)
	}
	if cfg.Email.SMTPPort != 587 {
		t.Fatalf("SMTP port = %d, want 587", cfg.Email.SMTPPort)
	}
	if cfg.Email.SignupOTPTTL != 10*time.Minute {
		t.Fatalf("signup OTP TTL = %s, want 10m", cfg.Email.SignupOTPTTL)
	}
	if cfg.Email.SendTimeout != 10*time.Second {
		t.Fatalf("email send timeout = %s, want 10s", cfg.Email.SendTimeout)
	}
	if cfg.Email.BrevoAPIURL != "https://api.brevo.com/v3/smtp/email" {
		t.Fatalf("Brevo API URL = %q", cfg.Email.BrevoAPIURL)
	}
	if cfg.Email.SignupOTPResendCooldown != time.Minute {
		t.Fatalf("signup OTP resend cooldown = %s, want 1m", cfg.Email.SignupOTPResendCooldown)
	}
	if cfg.Email.SignupOTPRequestWindow != time.Hour {
		t.Fatalf("signup OTP request window = %s, want 1h", cfg.Email.SignupOTPRequestWindow)
	}
	if cfg.Email.SignupOTPMaxEmailRequests != 5 {
		t.Fatalf("signup OTP max email requests = %d, want 5", cfg.Email.SignupOTPMaxEmailRequests)
	}
	if cfg.Email.SignupOTPMaxIPRequests != 20 {
		t.Fatalf("signup OTP max IP requests = %d, want 20", cfg.Email.SignupOTPMaxIPRequests)
	}
	if cfg.Email.SignupOTPCleanupInterval != 15*time.Minute {
		t.Fatalf("signup OTP cleanup interval = %s, want 15m", cfg.Email.SignupOTPCleanupInterval)
	}
	if cfg.Email.PasswordResetOTPTTL != 10*time.Minute {
		t.Fatalf("password reset OTP TTL = %s, want 10m", cfg.Email.PasswordResetOTPTTL)
	}
	if cfg.Email.PasswordResetOTPResendCooldown != time.Minute {
		t.Fatalf("password reset OTP resend cooldown = %s, want 1m", cfg.Email.PasswordResetOTPResendCooldown)
	}
	if cfg.Email.PasswordResetOTPRequestWindow != time.Hour {
		t.Fatalf("password reset OTP request window = %s, want 1h", cfg.Email.PasswordResetOTPRequestWindow)
	}
	if cfg.Email.PasswordResetOTPMaxEmailRequests != 5 {
		t.Fatalf("password reset OTP max email requests = %d, want 5", cfg.Email.PasswordResetOTPMaxEmailRequests)
	}
	if cfg.Email.PasswordResetOTPMaxIPRequests != 20 {
		t.Fatalf("password reset OTP max IP requests = %d, want 20", cfg.Email.PasswordResetOTPMaxIPRequests)
	}
}

func TestSLAIPaymentEnabledRequiresCallbackSecret(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SLAI_PAYMENT_ENABLED", "true")
	t.Setenv("SLAI_PAYMENT_CALLBACK_SECRET", "")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected callback secret requirement")
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
		"GOOGLE_CLIENT_ID",
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
		"STORAGE_DIR",
		"PAYMENT_PROOF_MAX_MB",
		"PAYMENT_QR_MAX_MB",
		"SLAI_PAYMENT_ENABLED",
		"SLAI_PAYMENT_BASE_URL",
		"SLAI_PAYMENT_API_KEY",
		"SLAI_PAYMENT_CALLBACK_BASE_URL",
		"SLAI_PAYMENT_CALLBACK_SECRET",
		"SLAI_PAYMENT_MERCHANT_PREFIX",
		"SLAI_PAYMENT_DEFAULT_EXPIRY",
		"SLAI_PAYMENT_HTTP_TIMEOUT",
		"SMTP_HOST",
		"SMTP_PORT",
		"SMTP_USERNAME",
		"SMTP_PASSWORD",
		"SMTP_FROM",
		"EMAIL_FROM",
		"BREVO_API_KEY",
		"BREVO_API_URL",
		"EMAIL_SEND_TIMEOUT",
		"SIGNUP_OTP_TTL",
		"SIGNUP_OTP_RESEND_COOLDOWN",
		"SIGNUP_OTP_REQUEST_WINDOW",
		"SIGNUP_OTP_MAX_EMAIL_REQUESTS",
		"SIGNUP_OTP_MAX_IP_REQUESTS",
		"SIGNUP_OTP_CLEANUP_INTERVAL",
		"PASSWORD_RESET_OTP_TTL",
		"PASSWORD_RESET_OTP_RESEND_COOLDOWN",
		"PASSWORD_RESET_OTP_REQUEST_WINDOW",
		"PASSWORD_RESET_OTP_MAX_EMAIL_REQUESTS",
		"PASSWORD_RESET_OTP_MAX_IP_REQUESTS",
	} {
		unsetEnv(t, key)
	}
	t.Setenv("DATABASE_URL", "postgres://slai:slai@localhost:5432/slai?sslmode=disable")
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	oldValue, hadValue := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv(key, oldValue)
			return
		}
		_ = os.Unsetenv(key)
	})
}
