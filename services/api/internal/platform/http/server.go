package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/admin"
	"github.com/slai/slai/services/api/internal/auth"
	"github.com/slai/slai/services/api/internal/ledger"
	"github.com/slai/slai/services/api/internal/packages"
	"github.com/slai/slai/services/api/internal/payments"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	"github.com/slai/slai/services/api/internal/users"
)

type ServerConfig struct {
	Addr             string
	ReadinessTimeout time.Duration
	SessionSecret    string
	CookieSecure     bool
	SessionTTL       time.Duration
}

type Server struct {
	server           *http.Server
	db               *pgxpool.Pool
	log              *slog.Logger
	readinessTimeout time.Duration
	sessionTTL       time.Duration
	sessions         auth.SessionManager
	authService      auth.Service
	paymentService   payments.Service
	adminService     admin.Service
}

func NewServer(cfg ServerConfig, pool *pgxpool.Pool, logger *slog.Logger) *Server {
	sessions := auth.NewSessionManager(pool, cfg.SessionSecret, cfg.CookieSecure, cfg.SessionTTL)
	server := &Server{
		db:               pool,
		log:              logger,
		readinessTimeout: cfg.ReadinessTimeout,
		sessionTTL:       cfg.SessionTTL,
		sessions:         sessions,
		authService:      auth.NewService(pool, sessions),
		paymentService:   payments.NewService(pool),
		adminService:     admin.NewService(pool),
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
	mux.HandleFunc("GET /v1/admin/packages", server.adminListPackages)
	mux.HandleFunc("POST /v1/admin/packages", server.adminCreatePackage)
	mux.HandleFunc("PATCH /v1/admin/packages/{id}", server.adminUpdatePackage)
	mux.HandleFunc("POST /v1/admin/payments/manual-topup", server.adminManualTopUp)
	mux.HandleFunc("POST /v1/admin/ledger/adjustments", server.adminLedgerAdjustment)

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

func (s *Server) Shutdown(ctx context.Context) error {
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
