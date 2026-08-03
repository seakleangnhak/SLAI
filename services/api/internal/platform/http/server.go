package httpserver

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/admin"
	"github.com/slai/slai/services/api/internal/apikeys"
	"github.com/slai/slai/services/api/internal/auth"
	"github.com/slai/slai/services/api/internal/config"
	"github.com/slai/slai/services/api/internal/ledger"
	"github.com/slai/slai/services/api/internal/notifications"
	"github.com/slai/slai/services/api/internal/omniroute"
	"github.com/slai/slai/services/api/internal/packages"
	"github.com/slai/slai/services/api/internal/payments"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	"github.com/slai/slai/services/api/internal/slaipayment"
	"github.com/slai/slai/services/api/internal/usage"
	"github.com/slai/slai/services/api/internal/users"
)

type ServerConfig struct {
	Addr              string
	ReadinessTimeout  time.Duration
	SessionSecret     string
	CookieSecure      bool
	SessionTTL        time.Duration
	GoogleClientID    string
	APIKeyPepper      string
	APIKeyPrefix      string
	OmniRoute         config.OmniRouteConfig
	OmniRouteClient   omniroute.Client
	UsageSyncWorker   config.UsageSyncWorkerConfig
	Storage           config.StorageConfig
	SLAIPayment       config.SLAIPaymentConfig
	SLAIPaymentClient slaipayment.Client
	GoogleVerifier    auth.GoogleIdentityVerifier
	Email             config.EmailConfig
	EmailSender       auth.EmailSender
}

type Server struct {
	server                   *http.Server
	db                       *pgxpool.Pool
	log                      *slog.Logger
	readinessTimeout         time.Duration
	sessionTTL               time.Duration
	sessions                 auth.SessionManager
	authService              auth.Service
	paymentService           payments.Service
	adminService             admin.Service
	apiKeyService            apikeys.Service
	usageService             usage.Service
	omniRouteCfg             config.OmniRouteConfig
	usageSyncCfg             config.UsageSyncWorkerConfig
	usageSyncExecutor        *usage.SyncExecutor
	usageSyncStatus          *usage.SyncStatusTracker
	usageSyncWorker          *usage.SyncWorker
	slaiPaymentCfg           config.SLAIPaymentConfig
	storageDir               string
	paymentProofMax          int64
	paymentQRMax             int64
	signupOTPCleanupInterval time.Duration
}

func NewServer(cfg ServerConfig, pool *pgxpool.Pool, logger *slog.Logger) *Server {
	sessions := auth.NewSessionManager(pool, cfg.SessionSecret, cfg.CookieSecure, cfg.SessionTTL)
	omniRouteClient := cfg.OmniRouteClient
	if omniRouteClient == nil {
		omniRouteClient = omniroute.NewStubClient(cfg.OmniRoute, logger)
	}
	emailSender := cfg.EmailSender
	if emailSender == nil {
		if strings.TrimSpace(cfg.Email.BrevoAPIKey) != "" {
			emailSender = auth.NewBrevoEmailSender(auth.BrevoConfig{
				APIKey:  cfg.Email.BrevoAPIKey,
				APIURL:  cfg.Email.BrevoAPIURL,
				From:    cfg.Email.SMTPFrom,
				Timeout: cfg.Email.SendTimeout,
			})
		} else if strings.TrimSpace(cfg.Email.SMTPHost) != "" {
			emailSender = auth.NewSMTPEmailSender(auth.SMTPConfig{
				Host:     cfg.Email.SMTPHost,
				Port:     cfg.Email.SMTPPort,
				Username: cfg.Email.SMTPUsername,
				Password: cfg.Email.SMTPPassword,
				From:     cfg.Email.SMTPFrom,
				Timeout:  cfg.Email.SendTimeout,
			})
		} else {
			emailSender = auth.NoopEmailSender{Log: logger}
		}
	}

	apiKeyService := apikeys.NewService(pool, apikeys.Config{
		Pepper:           cfg.APIKeyPepper,
		Prefix:           cfg.APIKeyPrefix,
		OmniRouteEnabled: cfg.OmniRoute.Enabled,
	}, omniRouteClient, logger)
	notificationService := notifications.NewService(pool, emailSender, notifications.Config{
		Enabled:        cfg.Email.LowBalanceAlertsEnabled,
		ThresholdUnits: cfg.Email.LowBalanceAlertThresholdUnits,
	})
	usageService := usage.NewService(pool, omniRouteClient, cfg.OmniRoute, logger).
		WithNotifications(notificationService)
	usageSyncStatus := usage.NewSyncStatusTracker(cfg.UsageSyncWorker.Enabled)
	usageSyncExecutor := usage.NewSyncExecutor(
		usageService,
		usage.PostgresAdvisoryLocker{Pool: pool},
		cfg.UsageSyncWorker,
		usageSyncStatus,
		logger,
	)
	usageSyncWorker := usage.NewSyncWorker(cfg.UsageSyncWorker, cfg.OmniRoute.Enabled, usageSyncExecutor, usageSyncStatus, logger)
	autoResumeAPIKey := func(ctx context.Context, userID string, balance ledger.Balance) {
		if balance.AvailableUnits <= 0 {
			return
		}
		if _, err := apiKeyService.ResumeAPIKey(ctx, userID); err != nil {
			if errors.Is(err, apikeys.ErrNotFound) || errors.Is(err, apikeys.ErrInsufficientBalance) {
				return
			}
			logger.Warn("api key auto-resume after credit top-up failed", "user_id", userID, "error", err)
		}
	}
	paymentService := payments.NewService(pool, payments.ProviderConfig{
		SLAIPaymentEnabled:         cfg.SLAIPayment.Enabled,
		SLAIPaymentCallbackBaseURL: cfg.SLAIPayment.CallbackBaseURL,
		SLAIPaymentMerchantPrefix:  cfg.SLAIPayment.MerchantPrefix,
		SLAIPaymentDefaultExpiry:   cfg.SLAIPayment.DefaultExpiry,
	}).WithSLAIPaymentClient(cfg.SLAIPaymentClient).WithAutoResumeAPIKey(autoResumeAPIKey)

	authService := auth.NewService(pool, sessions).
		WithEmailSender(emailSender).
		WithSignupOTPTTL(cfg.Email.SignupOTPTTL).
		WithSignupOTPControls(
			cfg.Email.SignupOTPResendCooldown,
			cfg.Email.SignupOTPRequestWindow,
			cfg.Email.SignupOTPMaxEmailRequests,
			cfg.Email.SignupOTPMaxIPRequests,
		).
		WithPasswordResetOTPTTL(cfg.Email.PasswordResetOTPTTL).
		WithPasswordResetOTPControls(
			cfg.Email.PasswordResetOTPResendCooldown,
			cfg.Email.PasswordResetOTPRequestWindow,
			cfg.Email.PasswordResetOTPMaxEmailRequests,
			cfg.Email.PasswordResetOTPMaxIPRequests,
		).
		WithPasswordChangedAlerts(cfg.Email.PasswordChangedAlertsEnabled)
	if strings.TrimSpace(cfg.GoogleClientID) != "" {
		authService = authService.WithGoogleVerifier(auth.NewGoogleIDTokenVerifier(cfg.GoogleClientID))
	}
	if cfg.GoogleVerifier != nil {
		authService = authService.WithGoogleVerifier(cfg.GoogleVerifier)
	}

	server := &Server{
		db:                       pool,
		log:                      logger,
		readinessTimeout:         cfg.ReadinessTimeout,
		sessionTTL:               cfg.SessionTTL,
		sessions:                 sessions,
		authService:              authService,
		paymentService:           paymentService,
		adminService:             admin.NewService(pool),
		apiKeyService:            apiKeyService,
		usageService:             usageService,
		omniRouteCfg:             cfg.OmniRoute,
		usageSyncCfg:             cfg.UsageSyncWorker,
		usageSyncExecutor:        usageSyncExecutor,
		usageSyncStatus:          usageSyncStatus,
		usageSyncWorker:          usageSyncWorker,
		slaiPaymentCfg:           cfg.SLAIPayment,
		storageDir:               cfg.Storage.Dir,
		paymentProofMax:          int64(cfg.Storage.PaymentProofMaxMB) * 1024 * 1024,
		paymentQRMax:             int64(cfg.Storage.PaymentQRMaxMB) * 1024 * 1024,
		signupOTPCleanupInterval: cfg.Email.SignupOTPCleanupInterval,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.healthz)
	mux.HandleFunc("GET /readyz", server.readyz)
	mux.HandleFunc("GET /", server.root)
	mux.HandleFunc("POST /v1/auth/signup", server.signup)
	mux.HandleFunc("POST /v1/auth/signup/verify", server.verifySignupEmail)
	mux.HandleFunc("POST /v1/auth/password-reset/request", server.requestPasswordReset)
	mux.HandleFunc("POST /v1/auth/password-reset/confirm", server.confirmPasswordReset)
	mux.HandleFunc("POST /v1/auth/login", server.login)
	mux.HandleFunc("POST /v1/auth/google", server.googleAuth)
	mux.HandleFunc("POST /v1/auth/logout", server.logout)
	mux.HandleFunc("GET /v1/me", server.me)
	mux.HandleFunc("POST /v1/me/password", server.changePassword)
	mux.HandleFunc("GET /v1/packages", server.listPublicPackages)
	mux.HandleFunc("GET /v1/balance", server.balance)
	mux.HandleFunc("GET /v1/ledger", server.ledgerEntries)
	mux.HandleFunc("GET /v1/payments", server.payments)
	mux.HandleFunc("GET /v1/payments/{id}", server.getPayment)
	mux.HandleFunc("POST /v1/payments/{id}/refresh", server.refreshPayment)
	mux.HandleFunc("POST /v1/payments/slai-payment/callback", server.slaiPaymentCallback)
	mux.HandleFunc("POST /v1/payments/{id}/proof", server.uploadPaymentProof)
	mux.HandleFunc("GET /v1/payments/{id}/proof", server.getPaymentProof)
	mux.HandleFunc("POST /v1/checkout/package/{package_id}", server.checkoutPackage)
	mux.HandleFunc("GET /v1/payment-settings/bakong-khqr", server.bakongPaymentSettings)
	mux.HandleFunc("GET /v1/payment-settings/bakong-khqr/khqr-image", server.bakongKHQRImage)
	mux.HandleFunc("GET /v1/usage", server.listUsage)
	mux.HandleFunc("GET /v1/api-key", server.getAPIKey)
	mux.HandleFunc("POST /v1/api-key", server.createAPIKey)
	mux.HandleFunc("POST /v1/api-key/rotate", server.rotateAPIKey)
	mux.HandleFunc("DELETE /v1/api-key", server.revokeAPIKey)
	mux.HandleFunc("GET /v1/admin/dashboard", server.adminDashboard)
	mux.HandleFunc("GET /v1/admin/packages", server.adminListPackages)
	mux.HandleFunc("POST /v1/admin/packages", server.adminCreatePackage)
	mux.HandleFunc("PATCH /v1/admin/packages/{id}", server.adminUpdatePackage)
	mux.HandleFunc("GET /v1/admin/payment-settings/bakong-khqr", server.adminGetBakongPaymentSettings)
	mux.HandleFunc("GET /v1/admin/payment-settings/bakong-khqr/provider-status", server.adminGetBakongPaymentProviderStatus)
	mux.HandleFunc("PATCH /v1/admin/payment-settings/bakong-khqr", server.adminUpdateBakongPaymentSettings)
	mux.HandleFunc("POST /v1/admin/payment-settings/bakong-khqr/khqr-image", server.adminUploadBakongKHQRImage)
	mux.HandleFunc("GET /v1/admin/payments", server.adminListPayments)
	mux.HandleFunc("GET /v1/admin/payments/{id}", server.adminGetPayment)
	mux.HandleFunc("GET /v1/admin/payments/{id}/proof", server.adminGetPaymentProof)
	mux.HandleFunc("POST /v1/admin/payments/{id}/approve", server.adminApprovePayment)
	mux.HandleFunc("POST /v1/admin/payments/{id}/reject", server.adminRejectPayment)
	mux.HandleFunc("POST /v1/admin/payments/manual-topup", server.adminManualTopUp)
	mux.HandleFunc("POST /v1/admin/ledger/adjustments", server.adminLedgerAdjustment)
	mux.HandleFunc("POST /v1/internal/usage/mock-event", server.ingestMockUsageEvent)
	mux.HandleFunc("POST /v1/internal/omniroute/api-keys/provision", server.provisionOmniRouteAPIKey)
	mux.HandleFunc("POST /v1/admin/usage/sync", server.adminSyncUsage)
	mux.HandleFunc("GET /v1/admin/usage/sync-status", server.adminUsageSyncStatus)
	mux.HandleFunc("GET /v1/admin/usage", server.adminListUsage)
	mux.HandleFunc("GET /v1/admin/audit-logs", server.adminListAuditLogs)
	mux.HandleFunc("GET /v1/admin/users", server.adminListUsers)
	mux.HandleFunc("GET /v1/admin/users/{id}", server.adminGetUser)
	mux.HandleFunc("PATCH /v1/admin/users/{id}/status", server.adminUpdateUserStatus)
	mux.HandleFunc("GET /v1/admin/users/{id}/api-key", server.adminGetUserAPIKey)
	mux.HandleFunc("POST /v1/admin/users/{id}/api-key/suspend", server.adminSuspendUserAPIKey)
	mux.HandleFunc("POST /v1/admin/users/{id}/api-key/resume", server.adminResumeUserAPIKey)
	mux.HandleFunc("POST /v1/admin/users/{id}/api-key/revoke", server.adminRevokeUserAPIKey)

	server.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return server
}

func (s *Server) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func (s *Server) StartUsageSyncWorker(ctx context.Context) bool {
	if s == nil || s.usageSyncWorker == nil {
		return false
	}
	return s.usageSyncWorker.Start(ctx)
}

func (s *Server) StartSignupOTPCleanupWorker(ctx context.Context) bool {
	if s == nil || s.signupOTPCleanupInterval <= 0 {
		return false
	}
	go func() {
		if err := s.authService.CleanupExpiredSignupOTPState(ctx); err != nil {
			s.log.Warn("signup OTP cleanup failed", "error", err)
		}

		ticker := time.NewTicker(s.signupOTPCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.authService.CleanupExpiredSignupOTPState(ctx); err != nil {
					s.log.Warn("signup OTP cleanup failed", "error", err)
				}
			}
		}
	}()
	return true
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.usageSyncWorker != nil {
		s.usageSyncWorker.Stop(ctx)
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.server.Handler.ServeHTTP(w, r)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK\n"))
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.readinessTimeout)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		s.log.Warn("database readiness check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"reason": "database_unavailable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) root(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "slai-api",
		"status":  "ok",
	})
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	verification, err := s.authService.Signup(r.Context(), req.Email, req.Password, clientIP(r))
	if err != nil {
		if errors.Is(err, auth.ErrEmailDeliveryFailed) {
			s.log.Warn("signup verification email delivery failed", "error", err)
		}
		writeSignupError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"verification": verification})
}

func (s *Server) verifySignupEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		OTP   string `json:"otp"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	user, token, err := s.authService.VerifySignupEmailOTP(r.Context(), req.Email, req.OTP)
	if err != nil {
		writeSignupError(w, err)
		return
	}

	s.sessions.SetCookie(w, token, time.Now().UTC().Add(s.sessionTTL))
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	verification, err := s.authService.RequestPasswordReset(r.Context(), req.Email, clientIP(r))
	if err != nil {
		if errors.Is(err, auth.ErrEmailDeliveryFailed) {
			s.log.Warn("password reset email delivery failed", "error", err)
		}
		writePasswordResetError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"verification": verification})
}

func (s *Server) confirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		OTP      string `json:"otp"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if err := s.authService.ResetPassword(r.Context(), req.Email, req.OTP, req.Password); err != nil {
		writePasswordResetError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeSignupError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "signup_failed"
	messageErr := err
	switch {
	case errors.Is(err, auth.ErrEmailAlreadyRegistered):
		status = http.StatusConflict
		code = "email_already_registered"
		messageErr = nil
	case errors.Is(err, auth.ErrEmailDeliveryFailed):
		status = http.StatusBadGateway
		code = "email_delivery_failed"
		messageErr = nil
	case errors.Is(err, auth.ErrInvalidVerificationCode):
		code = "invalid_verification_code"
		messageErr = nil
	case errors.Is(err, auth.ErrVerificationCodeExpired):
		code = "verification_code_expired"
		messageErr = nil
	case errors.Is(err, auth.ErrTooManyOTPAttempts):
		status = http.StatusTooManyRequests
		code = "too_many_verification_attempts"
		messageErr = nil
	case errors.Is(err, auth.ErrTooManyOTPRequests):
		status = http.StatusTooManyRequests
		code = "too_many_verification_code_requests"
		messageErr = nil
	case errors.Is(err, auth.ErrOTPResendTooSoon):
		status = http.StatusTooManyRequests
		code = "verification_code_requested_too_soon"
		messageErr = nil
	}
	var rateLimitErr auth.OTPRateLimitError
	if errors.As(err, &rateLimitErr) && rateLimitErr.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(rateLimitErr.RetryAfter.Seconds()+0.999)))
	}
	writeError(w, status, code, messageErr)
}

func writePasswordResetError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "password_reset_failed"
	messageErr := err
	switch {
	case errors.Is(err, auth.ErrEmailDeliveryFailed):
		status = http.StatusBadGateway
		code = "email_delivery_failed"
		messageErr = nil
	case errors.Is(err, auth.ErrInvalidVerificationCode):
		code = "invalid_verification_code"
		messageErr = nil
	case errors.Is(err, auth.ErrVerificationCodeExpired):
		code = "verification_code_expired"
		messageErr = nil
	case errors.Is(err, auth.ErrTooManyOTPAttempts):
		status = http.StatusTooManyRequests
		code = "too_many_verification_attempts"
		messageErr = nil
	case errors.Is(err, auth.ErrTooManyOTPRequests):
		status = http.StatusTooManyRequests
		code = "too_many_verification_code_requests"
		messageErr = nil
	case errors.Is(err, auth.ErrOTPResendTooSoon):
		status = http.StatusTooManyRequests
		code = "verification_code_requested_too_soon"
		messageErr = nil
	}
	var rateLimitErr auth.OTPRateLimitError
	if errors.As(err, &rateLimitErr) && rateLimitErr.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(rateLimitErr.RetryAfter.Seconds()+0.999)))
	}
	writeError(w, status, code, messageErr)
}

func writeChangePasswordError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "password_change_failed"
	messageErr := err
	switch {
	case errors.Is(err, auth.ErrInvalidCurrentPassword):
		status = http.StatusUnauthorized
		code = "invalid_current_password"
		messageErr = nil
	case errors.Is(err, auth.ErrPasswordAuthUnavailable):
		status = http.StatusConflict
		code = "password_auth_unavailable"
		messageErr = nil
	}
	writeError(w, status, code, messageErr)
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"CF-Connecting-IP", "X-Real-IP", "X-Forwarded-For"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		if header == "X-Forwarded-For" {
			parts := strings.Split(value, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		return value
	}
	host := strings.TrimSpace(r.RemoteAddr)
	if remoteHost, _, err := net.SplitHostPort(host); err == nil {
		return strings.Trim(remoteHost, "[]")
	}
	return host
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	user, token, err := s.authService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", nil)
		return
	}

	s.sessions.SetCookie(w, token, time.Now().UTC().Add(s.sessionTTL))
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) googleAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Credential string `json:"credential"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	user, token, err := s.authService.GoogleLogin(r.Context(), req.Credential)
	if err != nil {
		status := http.StatusUnauthorized
		code := "invalid_google_credential"
		if errors.Is(err, auth.ErrGoogleAuthDisabled) {
			status = http.StatusServiceUnavailable
			code = "google_auth_not_configured"
		} else if errors.Is(err, auth.ErrInvalidCredentials) {
			code = "invalid_credentials"
		}
		writeError(w, status, code, nil)
		return
	}

	s.sessions.SetCookie(w, token, time.Now().UTC().Add(s.sessionTTL))
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err == nil {
		if revokeErr := s.sessions.Revoke(r.Context(), cookie.Value); revokeErr != nil {
			s.log.Warn("session revoke failed", "error", revokeErr)
		}
	}
	s.sessions.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if err := s.authService.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword); err != nil {
		writeChangePasswordError(w, err)
		return
	}

	s.sessions.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listPublicPackages(w http.ResponseWriter, r *http.Request) {
	packages, err := packages.NewRepository(s.db).ListActive(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "packages_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": packages})
}

func (s *Server) balance(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireBalanceUserID(w, r)
	if !ok {
		return
	}
	balance, err := ledger.NewService(s.db).GetBalance(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "balance_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"balance": balance})
}

func (s *Server) ledgerEntries(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit := queryLimit(r, 50)
	entries, err := ledger.NewService(s.db).ListEntries(r.Context(), user.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ledger_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ledger": entries})
}

func (s *Server) payments(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	items, err := s.paymentService.ListForUser(r.Context(), user.ID, queryLimit(r, 50), queryOffset(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "payments_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payments": items})
}

func (s *Server) getPayment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	payment, err := s.paymentService.GetForUser(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment": payment})
}

func (s *Server) refreshPayment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	payment, err := s.paymentService.RefreshForUser(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment": payment})
}

func (s *Server) slaiPaymentCallback(w http.ResponseWriter, r *http.Request) {
	body, err := s.readAndVerifySLAIPaymentCallback(w, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_payment_callback_signature", err)
		return
	}
	var payload slaipayment.CallbackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payment_callback", err)
		return
	}
	event := strings.TrimSpace(payload.Event)
	if payload.Payment.ID == "" {
		writeError(w, http.StatusBadRequest, "invalid_payment_callback", payments.ErrPaymentCallbackInvalid)
		return
	}
	switch event {
	case "payment.paid":
	case "payment.expired":
		if strings.TrimSpace(payload.Payment.Status) == "" {
			payload.Payment.Status = "EXPIRED"
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid_payment_callback", payments.ErrPaymentCallbackInvalid)
		return
	}
	if headerID := strings.TrimSpace(r.Header.Get("X-SLAI-Payment-ID")); headerID == "" || headerID != payload.Payment.ID {
		writeError(w, http.StatusUnauthorized, "invalid_payment_callback_signature", payments.ErrPaymentCallbackInvalid)
		return
	}
	result, err := s.paymentService.ApplySLAIPayment(r.Context(), payload.Payment, true)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) checkoutPackage(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	result, err := s.paymentService.CheckoutPackage(r.Context(), user.ID, r.PathValue("package_id"))
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) uploadPaymentProof(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	file, err := s.storeUploadedFile(w, r, "payment-proofs", s.paymentProofMax, map[string]bool{
		"image/png":       true,
		"image/jpeg":      true,
		"image/webp":      true,
		"application/pdf": true,
	})
	if err != nil {
		writePaymentError(w, err)
		return
	}
	transactionRef := optionalString(r.FormValue("transaction_ref"))
	note := optionalString(r.FormValue("note"))
	payment, err := s.paymentService.UploadProof(r.Context(), user.ID, r.PathValue("id"), payments.ProofUploadInput{
		TransactionRef: transactionRef,
		Note:           note,
		File:           file,
	})
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment": payment})
}

func (s *Server) getPaymentProof(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	proof, err := s.paymentService.LatestProofForUser(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		writePaymentError(w, err)
		return
	}
	s.serveStoredFile(w, proof.FilePath, proof.FileMIME, proof.FileName)
}

func (s *Server) bakongPaymentSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}
	settings, err := s.paymentService.GetSettings(r.Context(), payments.ProviderBakongKHQR)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) bakongKHQRImage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}
	settings, err := s.paymentService.GetSettings(r.Context(), payments.ProviderBakongKHQR)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	if settings.KHQRImagePath == nil || settings.KHQRImageMIME == nil {
		writeError(w, http.StatusNotFound, "khqr_image_not_configured", payments.ErrPaymentSettingsIncomplete)
		return
	}
	s.serveStoredFile(w, *settings.KHQRImagePath, *settings.KHQRImageMIME, "bakong-khqr")
}

func (s *Server) adminGetBakongPaymentSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	settings, err := s.paymentService.GetSettings(r.Context(), payments.ProviderBakongKHQR)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) adminGetBakongPaymentProviderStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	status := payments.PaymentProviderStatus{
		Provider:                  payments.ProviderBakongKHQR,
		Mode:                      "automatic_slai_payment",
		Enabled:                   s.slaiPaymentCfg.Enabled,
		BaseURLConfigured:         strings.TrimSpace(s.slaiPaymentCfg.BaseURL) != "",
		APIKeyConfigured:          strings.TrimSpace(s.slaiPaymentCfg.APIKey) != "",
		CallbackBaseURLConfigured: strings.TrimSpace(s.slaiPaymentCfg.CallbackBaseURL) != "",
		CallbackSecretConfigured:  strings.TrimSpace(s.slaiPaymentCfg.CallbackSecret) != "",
		MerchantPrefix:            s.slaiPaymentCfg.MerchantPrefix,
		DefaultExpirySeconds:      int64(s.slaiPaymentCfg.DefaultExpiry.Seconds()),
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider_status": status})
}

func (s *Server) adminUpdateBakongPaymentSettings(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req payments.PaymentSettingsInput
	if !decodeJSON(w, r, &req) {
		return
	}
	settings, err := s.paymentService.UpdateSettings(r.Context(), adminUser.ID, payments.ProviderBakongKHQR, req)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) adminUploadBakongKHQRImage(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	file, err := s.storeUploadedFile(w, r, "payment-settings", s.paymentQRMax, map[string]bool{
		"image/png":  true,
		"image/jpeg": true,
		"image/webp": true,
	})
	if err != nil {
		writePaymentError(w, err)
		return
	}
	settings, err := s.paymentService.UpdateKHQRImage(r.Context(), adminUser.ID, payments.ProviderBakongKHQR, file)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) adminListPayments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	filter, err := paymentFilterFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payment_filter", err)
		return
	}
	result, err := s.paymentService.ListAdmin(r.Context(), filter)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) adminGetPayment(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	detail, err := s.paymentService.GetAdmin(r.Context(), r.PathValue("id"))
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment": detail})
}

func (s *Server) adminGetPaymentProof(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	proof, err := s.paymentService.LatestProofForAdmin(r.Context(), r.PathValue("id"))
	if err != nil {
		writePaymentError(w, err)
		return
	}
	s.serveStoredFile(w, proof.FilePath, proof.FileMIME, proof.FileName)
}

func (s *Server) adminApprovePayment(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req payments.ApproveInput
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.paymentService.Approve(r.Context(), adminUser.ID, r.PathValue("id"), req)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) adminRejectPayment(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req payments.RejectInput
	if !decodeJSON(w, r, &req) {
		return
	}
	payment, err := s.paymentService.Reject(r.Context(), adminUser.ID, r.PathValue("id"), req)
	if err != nil {
		writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment": payment})
}

func (s *Server) listUsage(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	filter, err := usageFilterFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_usage_filter", err)
		return
	}
	filter.UserID = &user.ID

	events, err := s.usageService.ListEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "usage_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usage": events})
}

func (s *Server) ingestMockUsageEvent(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	var req struct {
		APIKeyID        string     `json:"api_key_id"`
		ExternalEventID string     `json:"external_event_id"`
		Model           string     `json:"model"`
		Provider        string     `json:"provider"`
		InputTokens     int64      `json:"input_tokens"`
		OutputTokens    int64      `json:"output_tokens"`
		OccurredAt      *time.Time `json:"occurred_at"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	occurredAt := time.Now().UTC()
	if req.OccurredAt != nil {
		occurredAt = req.OccurredAt.UTC()
	}
	input := usage.IngestInput{
		ExternalEventID: strings.TrimSpace(req.ExternalEventID),
		APIKeyID:        optionalString(req.APIKeyID),
		Model:           optionalString(req.Model),
		Provider:        optionalString(req.Provider),
		InputTokens:     req.InputTokens,
		OutputTokens:    req.OutputTokens,
		OccurredAt:      occurredAt,
		Raw: map[string]any{
			"api_key_id":        req.APIKeyID,
			"external_event_id": req.ExternalEventID,
			"model":             req.Model,
			"provider":          req.Provider,
			"input_tokens":      req.InputTokens,
			"output_tokens":     req.OutputTokens,
			"occurred_at":       occurredAt.Format(time.RFC3339),
		},
	}

	result, err := s.usageService.IngestMockEvent(r.Context(), input)
	if err != nil {
		writeUsageError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Duplicate || result.Ignored {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) adminSyncUsage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	limit := queryLimit(r, s.usageSyncCfg.BatchLimit)
	result, err := s.usageSyncExecutor.RunWithLimit(r.Context(), limit)
	if err != nil {
		writeUsageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sync": result})
}

type usageSyncStatusResponse struct {
	usage.SyncStatus
	OmniRouteEnabled      bool   `json:"omniroute_enabled"`
	SyncMode              string `json:"sync_mode"`
	WorkerIntervalSeconds int    `json:"worker_interval_seconds"`
	BatchLimit            int    `json:"batch_limit"`
}

func (s *Server) adminUsageSyncStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	status := usageSyncStatusResponse{
		SyncStatus:            s.usageSyncStatus.Snapshot(),
		OmniRouteEnabled:      s.omniRouteCfg.Enabled,
		SyncMode:              s.omniRouteCfg.UsageSyncMode,
		WorkerIntervalSeconds: int(s.usageSyncCfg.Interval.Seconds()),
		BatchLimit:            s.usageSyncCfg.BatchLimit,
	}
	writeJSON(w, http.StatusOK, map[string]any{"sync_status": status})
}

func (s *Server) adminListUsage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	filter, err := usageFilterFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_usage_filter", err)
		return
	}
	events, err := s.usageService.ListEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "usage_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usage": events})
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	dashboard, err := admin.NewDashboardRepository(s.db).Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "admin_dashboard_failed", err)
		return
	}
	syncStatus := s.usageSyncStatus.Snapshot()
	dashboard.SyncStatus = admin.DashboardSyncStatus{
		WorkerEnabled:    syncStatus.WorkerEnabled,
		CurrentlyRunning: syncStatus.CurrentlyRunning,
		LastSuccessAt:    syncStatus.LastSuccessAt,
		LastError:        syncStatus.LastError,
	}
	writeJSON(w, http.StatusOK, dashboard)
}

func (s *Server) adminListAuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	filter, err := auditLogFilterFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_log_filter", err)
		return
	}
	result, err := admin.NewAuditLogRepository(s.db).List(r.Context(), filter)
	if err != nil {
		if errors.Is(err, admin.ErrInvalidAuditLogFilter) {
			writeError(w, http.StatusBadRequest, "invalid_audit_log_filter", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "audit_logs_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	key, err := s.apiKeyService.GetCurrentAPIKey(r.Context(), user.ID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, apikeys.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, "api_key_not_found", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_key": key.Public(s.apiKeyService.LocalDevMode())})
}

func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := s.apiKeyService.CreateAPIKey(r.Context(), user.ID, req.Name)
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	created, err := s.apiKeyService.RotateAPIKey(r.Context(), user.ID)
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	key, err := s.apiKeyService.RevokeAPIKey(r.Context(), user.ID)
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_key": key.Public(s.apiKeyService.LocalDevMode())})
}

func (s *Server) provisionOmniRouteAPIKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireOmniRouteManagementToken(w, r) {
		return
	}
	var req struct {
		RawAPIKey string `json:"raw_api_key"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	key, err := s.apiKeyService.ProvisionOmniRouteKeyForRawKey(r.Context(), req.RawAPIKey)
	if err != nil {
		writeProvisionOmniRouteAPIKeyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_key": key})
}

func (s *Server) adminListPackages(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	packages, err := packages.NewRepository(s.db).ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "packages_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": packages})
}

func (s *Server) adminCreatePackage(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req packages.CreatePackageInput
	if !decodeJSON(w, r, &req) {
		return
	}

	var pkg packages.Package
	err := platformdb.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		created, err := packages.NewRepository(tx).Create(r.Context(), req)
		if err != nil {
			return err
		}
		targetType := "credit_package"
		targetID := created.ID
		if err := admin.NewAuditLogger(tx).Log(r.Context(), adminUser.ID, "package_created", &targetType, &targetID, map[string]any{
			"name":        created.Name,
			"creditUnits": created.CreditUnits,
			"priceMinor":  created.PriceMinor,
			"currency":    created.Currency,
		}); err != nil {
			return err
		}
		pkg = created
		return nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "package_create_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"package": pkg})
}

func (s *Server) adminUpdatePackage(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req packages.UpdatePackageInput
	if !decodeJSON(w, r, &req) {
		return
	}

	var pkg packages.Package
	err := platformdb.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		updated, err := packages.NewRepository(tx).Update(r.Context(), r.PathValue("id"), req)
		if err != nil {
			return err
		}
		targetType := "credit_package"
		targetID := updated.ID
		if err := admin.NewAuditLogger(tx).Log(r.Context(), adminUser.ID, "package_updated", &targetType, &targetID, map[string]any{
			"name":   updated.Name,
			"active": updated.Active,
		}); err != nil {
			return err
		}
		pkg = updated
		return nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "package_update_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"package": pkg})
}

func (s *Server) adminManualTopUp(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req payments.ManualTopUpInput
	if !decodeJSON(w, r, &req) {
		return
	}
	applyIdempotencyHeader(r, &req.IdempotencyKey)

	result, err := s.paymentService.ManualTopUp(r.Context(), adminUser.ID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "manual_topup_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) adminLedgerAdjustment(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req admin.AdjustmentInput
	if !decodeJSON(w, r, &req) {
		return
	}
	applyIdempotencyHeader(r, &req.IdempotencyKey)

	result, err := s.adminService.AdjustCredits(r.Context(), adminUser.ID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "adjustment_failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	filter := users.AdminListFilter{
		Query:  r.URL.Query().Get("q"),
		Status: r.URL.Query().Get("status"),
		Role:   r.URL.Query().Get("role"),
		Limit:  queryLimit(r, 50),
		Offset: queryOffset(r),
	}
	result, err := users.NewAdminRepository(s.db).List(r.Context(), filter)
	if err != nil {
		if errors.Is(err, users.ErrInvalidAdminUserFilter) {
			writeError(w, http.StatusBadRequest, "invalid_user_filter", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "admin_users_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) adminGetUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	detail, err := users.NewAdminRepository(s.db).GetDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "admin_user_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) adminUpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}

	var req users.AdminStatusUpdateInput
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))

	var updated users.User
	err := platformdb.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		previousStatus, user, err := users.NewAdminRepository(tx).UpdateStatus(r.Context(), r.PathValue("id"), req.Status)
		if err != nil {
			return err
		}
		targetType := "user"
		targetID := user.ID
		if err := admin.NewAuditLogger(tx).Log(r.Context(), adminUser.ID, "user_status_updated", &targetType, &targetID, map[string]any{
			"previousStatus": previousStatus,
			"status":         user.Status,
		}); err != nil {
			return err
		}
		updated = user
		return nil
	})
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found", err)
			return
		}
		if errors.Is(err, users.ErrInvalidAdminUserFilter) {
			writeError(w, http.StatusBadRequest, "invalid_user_status", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "user_status_failed", err)
		return
	}

	if updated.Status == users.StatusSuspended {
		if _, err := s.apiKeyService.SuspendAPIKey(r.Context(), updated.ID); err != nil && !errors.Is(err, apikeys.ErrNotFound) {
			writeAPIKeyError(w, err)
			return
		}
	}

	detail, err := users.NewAdminRepository(s.db).GetDetail(r.Context(), updated.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "admin_user_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) adminGetUserAPIKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	key, err := s.apiKeyService.GetLatestAPIKey(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, apikeys.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, "api_key_not_found", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_key": key.Admin()})
}

func (s *Server) adminSuspendUserAPIKey(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	key, err := s.apiKeyService.SuspendAPIKey(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	if err := s.logAdminAPIKeyAction(r.Context(), adminUser.ID, "api_key_suspended", r.PathValue("id"), key.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "audit_log_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_key": key.Admin()})
}

func (s *Server) adminResumeUserAPIKey(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	key, err := s.apiKeyService.ResumeAPIKey(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	if err := s.logAdminAPIKeyAction(r.Context(), adminUser.ID, "api_key_resumed", r.PathValue("id"), key.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "audit_log_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_key": key.Admin()})
}

func (s *Server) adminRevokeUserAPIKey(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	key, err := s.apiKeyService.RevokeAPIKey(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIKeyError(w, err)
		return
	}
	if err := s.logAdminAPIKeyAction(r.Context(), adminUser.ID, "api_key_revoked", r.PathValue("id"), key.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "audit_log_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_key": key.Admin()})
}

func (s *Server) logAdminAPIKeyAction(ctx context.Context, adminID, action, userID, keyID string) error {
	targetType := "api_key"
	targetID := keyID
	return admin.NewAuditLogger(s.db).Log(ctx, adminID, action, &targetType, &targetID, map[string]any{"userId": userID})
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (users.User, bool) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthenticated", nil)
		return users.User{}, false
	}
	user, err := s.sessions.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", nil)
		return users.User{}, false
	}
	return user, true
}

func (s *Server) requireBalanceUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	_, hasAuthorization := r.Header[http.CanonicalHeaderKey("Authorization")]
	if hasAuthorization {
		rawKey := bearerToken(r.Header.Get("Authorization"))
		if rawKey == "" {
			writeError(w, http.StatusUnauthorized, "unauthenticated", nil)
			return "", false
		}
		userID, err := s.apiKeyService.AuthenticateBalanceKey(r.Context(), rawKey)
		if errors.Is(err, apikeys.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthenticated", nil)
			return "", false
		}
		if err != nil {
			s.log.Error("balance api key authentication failed", "error", err)
			writeError(w, http.StatusInternalServerError, "balance_auth_failed", nil)
			return "", false
		}
		return userID, true
	}

	user, ok := s.requireUser(w, r)
	if !ok {
		return "", false
	}
	return user.ID, true
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (users.User, bool) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return users.User{}, false
	}
	if !user.IsAdmin() {
		writeError(w, http.StatusForbidden, "admin_required", nil)
		return users.User{}, false
	}
	return user, true
}

func (s *Server) requireOmniRouteManagementToken(w http.ResponseWriter, r *http.Request) bool {
	configured := strings.TrimSpace(s.omniRouteCfg.ManagementToken)
	if !s.omniRouteCfg.Enabled || configured == "" {
		writeError(w, http.StatusNotImplemented, "omniroute_integration_disabled", omniroute.ErrNotImplemented)
		return false
	}
	provided := bearerToken(r.Header.Get("Authorization"))
	if provided == "" {
		writeError(w, http.StatusUnauthorized, "unauthenticated", nil)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) != 1 {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return false
	}
	return true
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if len(header) < len("Bearer ") || !strings.EqualFold(header[:len("Bearer ")], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[len("Bearer "):])
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, err error) {
	payload := map[string]any{"error": code}
	if err != nil {
		payload["message"] = err.Error()
	}
	writeJSON(w, status, payload)
}

func writeAPIKeyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apikeys.ErrNotFound):
		writeError(w, http.StatusNotFound, "api_key_not_found", err)
	case errors.Is(err, apikeys.ErrActiveKeyExists):
		writeError(w, http.StatusConflict, "active_api_key_exists", err)
	case errors.Is(err, apikeys.ErrInvalidName):
		writeError(w, http.StatusBadRequest, "invalid_api_key_name", err)
	case errors.Is(err, apikeys.ErrInsufficientBalance):
		writeError(w, http.StatusBadRequest, "insufficient_balance", err)
	default:
		writeError(w, http.StatusInternalServerError, "api_key_failed", err)
	}
}

func writeProvisionOmniRouteAPIKeyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apikeys.ErrNotFound):
		writeError(w, http.StatusNotFound, "api_key_not_found", err)
	case errors.Is(err, omniroute.ErrNotImplemented):
		writeError(w, http.StatusNotImplemented, "omniroute_integration_disabled", err)
	default:
		writeError(w, http.StatusInternalServerError, "api_key_failed", err)
	}
}

func queryLimit(r *http.Request, fallback int) int {
	limitRaw := r.URL.Query().Get("limit")
	if limitRaw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(limitRaw)
	if err != nil {
		return fallback
	}
	return limit
}

func queryOffset(r *http.Request) int {
	offsetRaw := r.URL.Query().Get("offset")
	if offsetRaw == "" {
		return 0
	}
	offset, err := strconv.Atoi(offsetRaw)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func usageFilterFromRequest(r *http.Request) (usage.ListFilter, error) {
	query := r.URL.Query()
	filter := usage.ListFilter{
		UserID:   optionalString(query.Get("user_id")),
		APIKeyID: optionalString(query.Get("api_key_id")),
		Model:    optionalString(query.Get("model")),
		Provider: optionalString(query.Get("provider")),
		Status:   optionalString(query.Get("status")),
		Limit:    queryLimit(r, 50),
		Offset:   queryOffset(r),
	}
	if from := strings.TrimSpace(query.Get("from")); from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return usage.ListFilter{}, err
		}
		filter.StartTime = &parsed
	}
	if to := strings.TrimSpace(query.Get("to")); to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return usage.ListFilter{}, err
		}
		filter.EndTime = &parsed
	}
	return filter, nil
}

func auditLogFilterFromRequest(r *http.Request) (admin.AuditLogFilter, error) {
	query := r.URL.Query()
	filter := admin.AuditLogFilter{
		AdminID:    query.Get("admin_id"),
		Action:     query.Get("action"),
		TargetType: query.Get("target_type"),
		TargetID:   query.Get("target_id"),
		Limit:      queryLimit(r, 50),
		Offset:     queryOffset(r),
	}
	if from := strings.TrimSpace(query.Get("from")); from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return admin.AuditLogFilter{}, err
		}
		filter.From = &parsed
	}
	if to := strings.TrimSpace(query.Get("to")); to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return admin.AuditLogFilter{}, err
		}
		filter.To = &parsed
	}
	return filter, nil
}

func paymentFilterFromRequest(r *http.Request) (payments.AdminPaymentFilter, error) {
	query := r.URL.Query()
	filter := payments.AdminPaymentFilter{
		Status:   query.Get("status"),
		UserID:   query.Get("user_id"),
		Provider: query.Get("provider"),
		Limit:    queryLimit(r, 50),
		Offset:   queryOffset(r),
	}
	if from := strings.TrimSpace(query.Get("from")); from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return payments.AdminPaymentFilter{}, err
		}
		filter.From = &parsed
	}
	if to := strings.TrimSpace(query.Get("to")); to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return payments.AdminPaymentFilter{}, err
		}
		filter.To = &parsed
	}
	return filter, nil
}

func (s *Server) readAndVerifySLAIPaymentCallback(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	secret := strings.TrimSpace(s.slaiPaymentCfg.CallbackSecret)
	if secret == "" {
		return nil, errors.New("payment callback secret is not configured")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	timestamp := strings.TrimSpace(r.Header.Get("X-SLAI-Payment-Timestamp"))
	signature := strings.TrimSpace(r.Header.Get("X-SLAI-Payment-Signature"))
	if timestamp == "" || signature == "" {
		return nil, errors.New("missing payment callback signature headers")
	}
	parsed, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, err
	}
	if delta := time.Since(time.Unix(parsed, 0)); delta > 5*time.Minute || delta < -5*time.Minute {
		return nil, errors.New("payment callback timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return nil, errors.New("payment callback signature mismatch")
	}
	return body, nil
}

func (s *Server) storeUploadedFile(w http.ResponseWriter, r *http.Request, subdir string, maxBytes int64, allowed map[string]bool) (payments.StoredFile, error) {
	if maxBytes <= 0 {
		maxBytes = 5 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024)
	if err := r.ParseMultipartForm(maxBytes + 1024); err != nil {
		return payments.StoredFile{}, payments.ErrInvalidPaymentProof
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return payments.StoredFile{}, payments.ErrInvalidPaymentProof
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return payments.StoredFile{}, err
	}
	if int64(len(data)) > maxBytes {
		return payments.StoredFile{}, payments.ErrInvalidPaymentProof
	}
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	if !allowed[mimeType] {
		return payments.StoredFile{}, payments.ErrInvalidPaymentProof
	}

	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	name := filepath.Base(header.Filename)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "upload"
	}
	ext := strings.ToLower(filepath.Ext(name))
	storedName := randomHex(16) + ext
	dir := filepath.Join(s.storageDir, subdir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return payments.StoredFile{}, err
	}
	path := filepath.Join(dir, storedName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return payments.StoredFile{}, err
	}
	return payments.StoredFile{Path: path, Name: name, MIME: mimeType, Size: int64(len(data)), SHA256: sha}, nil
}

func (s *Server) serveStoredFile(w http.ResponseWriter, path string, mimeType string, displayName string) {
	cleanStorage, err := filepath.Abs(s.storageDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_failed", err)
		return
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(cleanPath, cleanStorage+string(filepath.Separator)) {
		writeError(w, http.StatusNotFound, "file_not_found", nil)
		return
	}
	file, err := os.Open(cleanPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "file_not_found", nil)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusNotFound, "file_not_found", nil)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	if displayName != "" {
		w.Header().Set("Content-Disposition", `inline; filename="`+strings.ReplaceAll(displayName, `"`, "")+`"`)
	}
	http.ServeContent(w, rWithBackground(), displayName, stat.ModTime(), file)
}

func rWithBackground() *http.Request {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	return req
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func writePaymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, payments.ErrPaymentSettingsDisabled):
		writeError(w, http.StatusBadRequest, "payment_settings_disabled", err)
	case errors.Is(err, payments.ErrPaymentSettingsIncomplete):
		writeError(w, http.StatusBadRequest, "payment_settings_incomplete", err)
	case errors.Is(err, payments.ErrInvalidPaymentSettings):
		writeError(w, http.StatusBadRequest, "invalid_payment_settings", err)
	case errors.Is(err, payments.ErrPaymentProviderDisabled):
		writeError(w, http.StatusBadRequest, "payment_provider_disabled", err)
	case errors.Is(err, payments.ErrPaymentProviderMismatch):
		writeError(w, http.StatusConflict, "payment_provider_mismatch", err)
	case errors.Is(err, payments.ErrPaymentCallbackInvalid):
		writeError(w, http.StatusBadRequest, "invalid_payment_callback", err)
	case errors.Is(err, payments.ErrPaymentNotFound):
		writeError(w, http.StatusNotFound, "payment_not_found", err)
	case errors.Is(err, payments.ErrPaymentForbidden):
		writeError(w, http.StatusForbidden, "payment_forbidden", err)
	case errors.Is(err, payments.ErrInvalidPaymentProof):
		writeError(w, http.StatusBadRequest, "invalid_payment_proof", err)
	case errors.Is(err, payments.ErrPaymentReferenceRequired):
		writeError(w, http.StatusBadRequest, "payment_reference_required", err)
	case errors.Is(err, payments.ErrDuplicatePaymentReference):
		writeError(w, http.StatusConflict, "duplicate_payment_reference", err)
	case errors.Is(err, payments.ErrPaymentAlreadyPaid):
		writeError(w, http.StatusConflict, "payment_already_paid", err)
	case errors.Is(err, payments.ErrInvalidPaymentState):
		writeError(w, http.StatusBadRequest, "invalid_payment_state", err)
	case errors.Is(err, packages.ErrInvalidPackage):
		writeError(w, http.StatusBadRequest, "invalid_package", err)
	case errors.Is(err, slaipayment.ErrNotFound):
		writeError(w, http.StatusNotFound, "payment_provider_not_found", err)
	case errors.Is(err, slaipayment.ErrBadStatus), errors.Is(err, slaipayment.ErrInvalidPayload):
		writeError(w, http.StatusBadGateway, "payment_provider_failed", err)
	default:
		writeError(w, http.StatusInternalServerError, "payment_failed", err)
	}
}

func writeUsageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usage.ErrInvalidUsageEvent):
		writeError(w, http.StatusBadRequest, "invalid_usage_event", err)
	case errors.Is(err, usage.ErrPricingRuleNotFound):
		writeError(w, http.StatusBadRequest, "pricing_rule_not_found", err)
	case errors.Is(err, usage.ErrSyncNotImplemented):
		writeError(w, http.StatusNotImplemented, "omniroute_sync_not_implemented", err)
	case errors.Is(err, usage.ErrSyncLockHeld):
		writeError(w, http.StatusConflict, "usage_sync_lock_held", err)
	case errors.Is(err, usage.ErrSyncAlreadyRunning):
		writeError(w, http.StatusConflict, "usage_sync_already_running", err)
	default:
		writeError(w, http.StatusInternalServerError, "usage_failed", err)
	}
}

func applyIdempotencyHeader(r *http.Request, dst **string) {
	if dst == nil || (*dst != nil && **dst != "") {
		return
	}
	value := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if value == "" {
		return
	}
	*dst = &value
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
