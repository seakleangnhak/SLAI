package usage_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/slai/slai/services/api/internal/config"
	"github.com/slai/slai/services/api/internal/usage"
)

func TestSyncWorkerDisabledDoesNotStart(t *testing.T) {
	cfg := config.UsageSyncWorkerConfig{
		Enabled:    false,
		Interval:   time.Millisecond,
		StartDelay: time.Millisecond,
		BatchLimit: 10,
		LockKey:    "test_usage_sync_disabled",
	}
	status := usage.NewSyncStatusTracker(cfg.Enabled)
	svc := &fakeSyncService{started: make(chan struct{}, 1)}
	executor := usage.NewSyncExecutor(svc, &fakeAdvisoryLocker{acquired: true}, cfg, status, testLogger())
	worker := usage.NewSyncWorker(cfg, true, executor, status, testLogger())

	if worker.Start(context.Background()) {
		t.Fatal("disabled worker started")
	}
	select {
	case <-svc.started:
		t.Fatal("sync service was called for disabled worker")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestSyncWorkerEnabledCallsSyncOnInterval(t *testing.T) {
	cfg := config.UsageSyncWorkerConfig{
		Enabled:    true,
		Interval:   10 * time.Millisecond,
		StartDelay: time.Millisecond,
		BatchLimit: 7,
		LockKey:    "test_usage_sync_interval",
	}
	status := usage.NewSyncStatusTracker(cfg.Enabled)
	svc := &fakeSyncService{started: make(chan struct{}, 2), result: usage.SyncResult{Fetched: 1, Billed: 1}}
	executor := usage.NewSyncExecutor(svc, &fakeAdvisoryLocker{acquired: true}, cfg, status, testLogger())
	worker := usage.NewSyncWorker(cfg, true, executor, status, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !worker.Start(ctx) {
		t.Fatal("enabled worker did not start")
	}
	if worker.Start(ctx) {
		t.Fatal("worker started a second loop")
	}
	defer worker.Stop(context.Background())

	select {
	case <-svc.started:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("worker did not call sync service")
	}
	if got := svc.lastLimit(); got != 7 {
		t.Fatalf("sync limit = %d, want 7", got)
	}
}

func TestSyncExecutorSkipsWhenLockHeld(t *testing.T) {
	cfg := config.UsageSyncWorkerConfig{BatchLimit: 10, LockKey: "test_usage_sync_lock_held"}
	status := usage.NewSyncStatusTracker(true)
	svc := &fakeSyncService{result: usage.SyncResult{Fetched: 1}}
	locker := &fakeAdvisoryLocker{acquired: false}
	executor := usage.NewSyncExecutor(svc, locker, cfg, status, testLogger())

	_, err := executor.Run(context.Background())
	if !errors.Is(err, usage.ErrSyncLockHeld) {
		t.Fatalf("err = %v, want ErrSyncLockHeld", err)
	}
	if svc.callCount() != 0 {
		t.Fatalf("sync service calls = %d, want 0", svc.callCount())
	}
	if locker.releaseCountValue() != 0 {
		t.Fatalf("release count = %d, want 0", locker.releaseCountValue())
	}
}

func TestSyncExecutorReleasesLockAndUpdatesStatusAfterSuccess(t *testing.T) {
	cfg := config.UsageSyncWorkerConfig{BatchLimit: 10, LockKey: "test_usage_sync_success"}
	status := usage.NewSyncStatusTracker(true)
	svc := &fakeSyncService{result: usage.SyncResult{Fetched: 2, Billed: 1, Duplicate: 1}}
	locker := &fakeAdvisoryLocker{acquired: true}
	executor := usage.NewSyncExecutor(svc, locker, cfg, status, testLogger())

	result, err := executor.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 2 || result.Billed != 1 || result.Duplicate != 1 {
		t.Fatalf("result = %#v", result)
	}
	if locker.releaseCountValue() != 1 {
		t.Fatalf("release count = %d, want 1", locker.releaseCountValue())
	}
	snapshot := status.Snapshot()
	if snapshot.CurrentlyRunning || snapshot.LastSuccessAt == nil || snapshot.LastError != nil || snapshot.LastResult == nil {
		t.Fatalf("unexpected status after success: %#v", snapshot)
	}
}

func TestSyncExecutorReleasesLockAndUpdatesStatusAfterError(t *testing.T) {
	cfg := config.UsageSyncWorkerConfig{BatchLimit: 10, LockKey: "test_usage_sync_error"}
	status := usage.NewSyncStatusTracker(true)
	svcErr := errors.New("sync failed")
	svc := &fakeSyncService{result: usage.SyncResult{Fetched: 3, Failed: 1}, err: svcErr}
	locker := &fakeAdvisoryLocker{acquired: true}
	executor := usage.NewSyncExecutor(svc, locker, cfg, status, testLogger())

	result, err := executor.Run(context.Background())
	if !errors.Is(err, svcErr) {
		t.Fatalf("err = %v, want %v", err, svcErr)
	}
	if result.Fetched != 3 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	if locker.releaseCountValue() != 1 {
		t.Fatalf("release count = %d, want 1", locker.releaseCountValue())
	}
	snapshot := status.Snapshot()
	if snapshot.CurrentlyRunning || snapshot.LastError == nil || snapshot.LastSuccessAt != nil || snapshot.LastResult == nil {
		t.Fatalf("unexpected status after error: %#v", snapshot)
	}
}

func TestSyncExecutorPreventsSameProcessConcurrentRuns(t *testing.T) {
	cfg := config.UsageSyncWorkerConfig{BatchLimit: 10, LockKey: "test_usage_sync_same_process"}
	status := usage.NewSyncStatusTracker(true)
	unblock := make(chan struct{})
	svc := &fakeSyncService{started: make(chan struct{}, 1), unblock: unblock}
	executor := usage.NewSyncExecutor(svc, &fakeAdvisoryLocker{acquired: true}, cfg, status, testLogger())

	done := make(chan error, 1)
	go func() {
		_, err := executor.Run(context.Background())
		done <- err
	}()

	select {
	case <-svc.started:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("first sync did not start")
	}

	_, err := executor.Run(context.Background())
	if !errors.Is(err, usage.ErrSyncAlreadyRunning) {
		t.Fatalf("err = %v, want ErrSyncAlreadyRunning", err)
	}
	close(unblock)
	if err := <-done; err != nil {
		t.Fatalf("first sync failed: %v", err)
	}
}

type fakeSyncService struct {
	mu      sync.Mutex
	calls   int
	limits  []int
	result  usage.SyncResult
	err     error
	started chan struct{}
	unblock chan struct{}
}

func (s *fakeSyncService) SyncOmniRoute(ctx context.Context, limit int) (usage.SyncResult, error) {
	s.mu.Lock()
	s.calls++
	s.limits = append(s.limits, limit)
	s.mu.Unlock()
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.unblock != nil {
		select {
		case <-s.unblock:
		case <-ctx.Done():
			return usage.SyncResult{}, ctx.Err()
		}
	}
	return s.result, s.err
}

func (s *fakeSyncService) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *fakeSyncService) lastLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.limits) == 0 {
		return 0
	}
	return s.limits[len(s.limits)-1]
}

type fakeAdvisoryLocker struct {
	mu           sync.Mutex
	acquired     bool
	err          error
	attempts     int
	releaseCount int
}

func (l *fakeAdvisoryLocker) TryAcquireAdvisoryLock(context.Context, string) (bool, func(), error) {
	l.mu.Lock()
	l.attempts++
	acquired := l.acquired
	err := l.err
	l.mu.Unlock()
	if err != nil {
		return false, nil, err
	}
	if !acquired {
		return false, func() {}, nil
	}
	return true, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.releaseCount++
	}, nil
}

func (l *fakeAdvisoryLocker) releaseCountValue() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.releaseCount
}
