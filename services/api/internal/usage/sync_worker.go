package usage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/config"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var (
	ErrSyncLockHeld       = errors.New("usage sync lock is held by another process")
	ErrSyncAlreadyRunning = errors.New("usage sync is already running in this process")
)

type SyncService interface {
	SyncOmniRoute(ctx context.Context, limit int) (SyncResult, error)
}

type AdvisoryLocker interface {
	TryAcquireAdvisoryLock(ctx context.Context, key string) (bool, func(), error)
}

type PostgresAdvisoryLocker struct {
	Pool *pgxpool.Pool
}

func (l PostgresAdvisoryLocker) TryAcquireAdvisoryLock(ctx context.Context, key string) (bool, func(), error) {
	return platformdb.TryAcquireAdvisoryLock(ctx, l.Pool, key)
}

type SyncStatus struct {
	WorkerEnabled    bool        `json:"worker_enabled"`
	LastStartedAt    *time.Time  `json:"last_started_at"`
	LastFinishedAt   *time.Time  `json:"last_finished_at"`
	LastSuccessAt    *time.Time  `json:"last_success_at"`
	LastError        *string     `json:"last_error"`
	LastResult       *SyncResult `json:"last_result"`
	NextRunAt        *time.Time  `json:"next_run_at"`
	CurrentlyRunning bool        `json:"currently_running"`
}

type SyncStatusTracker struct {
	mu     sync.RWMutex
	status SyncStatus
}

func NewSyncStatusTracker(workerEnabled bool) *SyncStatusTracker {
	return &SyncStatusTracker{status: SyncStatus{WorkerEnabled: workerEnabled}}
}

func (t *SyncStatusTracker) Snapshot() SyncStatus {
	if t == nil {
		return SyncStatus{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

func (t *SyncStatusTracker) SetWorkerEnabled(enabled bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.WorkerEnabled = enabled
}

func (t *SyncStatusTracker) SetNextRunAt(next *time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.NextRunAt = next
}

func (t *SyncStatusTracker) MarkStarted(startedAt time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	startedAt = startedAt.UTC()
	t.status.LastStartedAt = &startedAt
	t.status.CurrentlyRunning = true
	t.status.LastError = nil
}

func (t *SyncStatusTracker) MarkFinished(finishedAt time.Time, result SyncResult, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	finishedAt = finishedAt.UTC()
	t.status.LastFinishedAt = &finishedAt
	t.status.LastResult = &result
	t.status.CurrentlyRunning = false
	if err != nil {
		message := err.Error()
		t.status.LastError = &message
		return
	}
	t.status.LastError = nil
	t.status.LastSuccessAt = &finishedAt
}

type SyncExecutor struct {
	service    SyncService
	locker     AdvisoryLocker
	status     *SyncStatusTracker
	lockKey    string
	batchLimit int
	logger     *slog.Logger
	localMu    sync.Mutex
}

func NewSyncExecutor(service SyncService, locker AdvisoryLocker, cfg config.UsageSyncWorkerConfig, status *SyncStatusTracker, logger *slog.Logger) *SyncExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.LockKey == "" {
		cfg.LockKey = "slai_usage_sync"
	}
	if cfg.BatchLimit <= 0 {
		cfg.BatchLimit = 100
	}
	return &SyncExecutor{
		service:    service,
		locker:     locker,
		status:     status,
		lockKey:    cfg.LockKey,
		batchLimit: cfg.BatchLimit,
		logger:     logger,
	}
}

func (e *SyncExecutor) Run(ctx context.Context) (SyncResult, error) {
	return e.RunWithLimit(ctx, 0)
}

func (e *SyncExecutor) RunWithLimit(ctx context.Context, limit int) (result SyncResult, err error) {
	if e == nil || e.service == nil {
		return SyncResult{}, ErrSyncNotImplemented
	}
	if limit <= 0 {
		limit = e.batchLimit
	}
	if !e.localMu.TryLock() {
		return SyncResult{}, ErrSyncAlreadyRunning
	}
	defer e.localMu.Unlock()

	if e.locker == nil {
		return SyncResult{}, errors.New("usage sync advisory locker is not configured")
	}
	acquired, release, err := e.locker.TryAcquireAdvisoryLock(ctx, e.lockKey)
	if err != nil {
		return SyncResult{}, err
	}
	if !acquired {
		return SyncResult{}, ErrSyncLockHeld
	}
	defer release()

	e.status.MarkStarted(time.Now())
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("usage sync panic: %v", recovered)
		}
		e.status.MarkFinished(time.Now(), result, err)
	}()

	result, err = e.service.SyncOmniRoute(ctx, limit)
	return result, err
}

type SyncWorker struct {
	cfg              config.UsageSyncWorkerConfig
	omniRouteEnabled bool
	executor         *SyncExecutor
	status           *SyncStatusTracker
	logger           *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewSyncWorker(cfg config.UsageSyncWorkerConfig, omniRouteEnabled bool, executor *SyncExecutor, status *SyncStatusTracker, logger *slog.Logger) *SyncWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.LockKey == "" {
		cfg.LockKey = "slai_usage_sync"
	}
	if cfg.BatchLimit <= 0 {
		cfg.BatchLimit = 100
	}
	if status != nil {
		status.SetWorkerEnabled(cfg.Enabled)
	}
	return &SyncWorker{cfg: cfg, omniRouteEnabled: omniRouteEnabled, executor: executor, status: status, logger: logger}
}

func (w *SyncWorker) Start(parent context.Context) bool {
	if w == nil || !w.cfg.Enabled {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return false
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.done = make(chan struct{})
	go w.loop(ctx)
	return true
}

func (w *SyncWorker) Stop(ctx context.Context) {
	if w == nil {
		return
	}
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	w.cancel = nil
	w.done = nil
	w.mu.Unlock()
	if cancel == nil || done == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (w *SyncWorker) loop(ctx context.Context) {
	defer close(w.done)
	w.logger.Info("usage sync worker started", "interval_seconds", int(w.cfg.Interval.Seconds()), "start_delay_seconds", int(w.cfg.StartDelay.Seconds()))
	defer func() {
		w.status.SetNextRunAt(nil)
		w.logger.Info("usage sync worker stopped")
	}()

	delay := w.cfg.StartDelay
	for {
		if delay > 0 {
			next := time.Now().Add(delay).UTC()
			w.status.SetNextRunAt(&next)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		} else {
			now := time.Now().UTC()
			w.status.SetNextRunAt(&now)
		}

		w.status.SetNextRunAt(nil)
		w.runTick(ctx)
		delay = w.cfg.Interval
	}
}

func (w *SyncWorker) runTick(ctx context.Context) {
	if !w.omniRouteEnabled {
		w.logger.Info("usage sync skipped because omniroute is disabled")
		return
	}

	w.logger.Info("usage sync started")
	result, err := w.executor.Run(ctx)
	if errors.Is(err, ErrSyncLockHeld) {
		w.logger.Info("usage sync skipped because lock held", "lock_key", w.cfg.LockKey)
		return
	}
	if errors.Is(err, ErrSyncAlreadyRunning) {
		w.logger.Info("usage sync skipped because another sync is running in this process")
		return
	}
	if err != nil {
		w.logger.Error("usage sync failed", "error", err, "fetched", result.Fetched, "billed", result.Billed, "duplicate", result.Duplicate, "ignored", result.Ignored, "failed", result.Failed, "suspended_keys", result.SuspendedKeys)
		return
	}
	w.logger.Info("usage sync completed", "fetched", result.Fetched, "billed", result.Billed, "duplicate", result.Duplicate, "ignored", result.Ignored, "failed", result.Failed, "suspended_keys", result.SuspendedKeys)
}
