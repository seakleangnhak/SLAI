package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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
	"github.com/slai/slai/services/api/internal/omniroute"
	"github.com/slai/slai/services/api/internal/packages"
	"github.com/slai/slai/services/api/internal/payments"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	"github.com/slai/slai/services/api/internal/usage"
	"github.com/slai/slai/services/api/internal/users"
)

type ServerConfig struct {
	Addr             string
	ReadinessTimeout time.Duration
	SessionSecret    string
	CookieSecure     bool
	SessionTTL       time.Duration
	APIKeyPepper     string
	APIKeyPrefix     string
	OmniRoute        config.OmniRouteConfig
	OmniRouteClient  omniroute.Client
	UsageSyncWorker  config.UsageSyncWorkerConfig
}

type Server struct {
	server            *http.Server
	db                *pgxpool.Pool
	log               *slog.Logger
	readinessTimeout  time.Duration
	sessionTTL        time.Duration
	sessions          auth.SessionManager
	authService       auth.Service
	paymentService    payments.Service
	adminService      admin.Service
	apiKeyService     apikeys.Service
	usageService      usage.Service
	omniRouteCfg      config.OmniRouteConfig
	usageSyncCfg      config.UsageSyncWorkerConfig
	usageSyncExecutor *usage.SyncExecutor
	usageSyncStatus   *usage.SyncStatusTracker
	usageSyncWorker   *usage.SyncWorker
}

func NewServer(cfg ServerConfig, pool *pgxpool.Pool, logger *slog.Logger) *Server {
	sessions := auth.NewSessionManager(pool, cfg.SessionSecret, cfg.CookieSecure, cfg.SessionTTL)
	omniRouteClient := cfg.OmniRouteClient
	if omniRouteClient == nil {
		omniRouteClient = omniroute.NewStubClient(cfg.OmniRoute, logger)
	}
	apiKeyService := apikeys.NewService(pool, apikeys.Config{
		Pepper:           cfg.APIKeyPepper,
		Prefix:           cfg.APIKeyPrefix,
		OmniRouteEnabled: cfg.OmniRoute.Enabled,
	}, omniRouteClient, logger)
	usageService := usage.NewService(pool, omniRouteClient, cfg.OmniRoute, logger)
	usageSyncStatus := usage.NewSyncStatusTracker(cfg.UsageSyncWorker.Enabled)
	usageSyncExecutor := usage.NewSyncExecutor(
		usageService,
		usage.PostgresAdvisoryLocker{Pool: pool},
		cfg.UsageSyncWorker,
		usageSyncStatus,
		logger,
	)
	usageSyncWorker := usage.NewSyncWorker(cfg.UsageSyncWorker, cfg.OmniRoute.Enabled, usageSyncExecutor, usageSyncStatus, logger)

	server := &Server{
		db:                pool,
		log:               logger,
		readinessTimeout:  cfg.ReadinessTimeout,
		sessionTTL:        cfg.SessionTTL,
		sessions:          sessions,
		authService:       auth.NewService(pool, sessions),
		paymentService:    payments.NewService(pool),
		adminService:      admin.NewService(pool),
		apiKeyService:     apiKeyService,
		usageService:      usageService,
		omniRouteCfg:      cfg.OmniRoute,
		usageSyncCfg:      cfg.UsageSyncWorker,
		usageSyncExecutor: usageSyncExecutor,
		usageSyncStatus:   usageSyncStatus,
		usageSyncWorker:   usageSyncWorker,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.healthz)
	mux.HandleFunc("GET /readyz", server.readyz)
	mux.HandleFunc("GET /", server.root)
	mux.HandleFunc("POST /v1/auth/signup", server.signup)
	mux.HandleFunc("POST /v1/auth/login", server.login)
	mux.HandleFunc("POST /v1/auth/logout", server.logout)
	mux.HandleFunc("GET /v1/me", server.me)
	mux.HandleFunc("GET /v1/packages", server.listPublicPackages)
	mux.HandleFunc("GET /v1/balance", server.balance)
	mux.HandleFunc("GET /v1/ledger", server.ledgerEntries)
	mux.HandleFunc("GET /v1/usage", server.listUsage)
	mux.HandleFunc("GET /v1/api-key", server.getAPIKey)
	mux.HandleFunc("POST /v1/api-key", server.createAPIKey)
	mux.HandleFunc("POST /v1/api-key/rotate", server.rotateAPIKey)
	mux.HandleFunc("DELETE /v1/api-key", server.revokeAPIKey)
	mux.HandleFunc("GET /v1/admin/dashboard", server.adminDashboard)
	mux.HandleFunc("GET /v1/admin/packages", server.adminListPackages)
	mux.HandleFunc("POST /v1/admin/packages", server.adminCreatePackage)
	mux.HandleFunc("PATCH /v1/admin/packages/{id}", server.adminUpdatePackage)
	mux.HandleFunc("POST /v1/admin/payments/manual-topup", server.adminManualTopUp)
	mux.HandleFunc("POST /v1/admin/ledger/adjustments", server.adminLedgerAdjustment)
	mux.HandleFunc("POST /v1/internal/usage/mock-event", server.ingestMockUsageEvent)
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

	user, token, err := s.authService.Signup(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signup_failed", err)
		return
	}

	s.sessions.SetCookie(w, token, time.Now().UTC().Add(s.sessionTTL))
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
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

func (s *Server) listPublicPackages(w http.ResponseWriter, r *http.Request) {
	packages, err := packages.NewRepository(s.db).ListActive(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "packages_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": packages})
}

func (s *Server) balance(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	balance, err := ledger.NewService(s.db).GetBalance(r.Context(), user.ID)
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

func (s *Server) listUsage(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	filter := usage.ListFilter{
		UserID: &user.ID,
		Limit:  queryLimit(r, 50),
		Offset: queryOffset(r),
	}
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
