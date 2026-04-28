package db

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

func AdvisoryLockKey(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(binary.BigEndian.Uint64(h.Sum(nil)))
}

func TryAcquireAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, key string) (bool, func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("acquire advisory lock connection: %w", err)
	}

	lockID := AdvisoryLockKey(key)
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockID).Scan(&acquired); err != nil {
		conn.Release()
		return false, nil, fmt.Errorf("try advisory lock: %w", err)
	}
	if !acquired {
		conn.Release()
		return false, func() {}, nil
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			defer conn.Release()
			var unlocked bool
			if err := conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID).Scan(&unlocked); err != nil {
				return
			}
		})
	}

	return true, release, nil
}
