package db

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Migrator struct {
	pool *pgxpool.Pool
	dir  string
	log  *slog.Logger
}

func NewMigrator(pool *pgxpool.Pool, dir string, logger *slog.Logger) Migrator {
	return Migrator{pool: pool, dir: dir, log: logger}
}

func (m Migrator) Up(ctx context.Context) error {
	if err := m.ensureSchemaMigrations(ctx); err != nil {
		return err
	}

	migrations, err := m.readMigrationFiles()
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		applied, err := m.isApplied(ctx, migration.version)
		if err != nil {
			return err
		}
		if applied {
			m.log.Debug("migration already applied", "version", migration.version)
			continue
		}

		if err := m.apply(ctx, migration); err != nil {
			return err
		}
	}

	m.log.Info("migrations complete", "count", len(migrations), "dir", m.dir)
	return nil
}

func (m Migrator) ensureSchemaMigrations(ctx context.Context) error {
	_, err := m.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func (m Migrator) isApplied(ctx context.Context, version string) (bool, error) {
	var applied bool
	err := m.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied)
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return applied, nil
}

func (m Migrator) apply(ctx context.Context, migration migrationFile) error {
	contents, err := os.ReadFile(migration.path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", migration.path, err)
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.version, err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, string(contents)); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.version, err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, migration.version); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.version, err)
	}

	m.log.Info("migration applied", "version", migration.version)
	return nil
}

func (m Migrator) readMigrationFiles() ([]migrationFile, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %s: %w", m.dir, err)
	}

	files := make([]migrationFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, migrationFile{
			version: strings.TrimSuffix(entry.Name(), ".sql"),
			path:    filepath.Join(m.dir, entry.Name()),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].version < files[j].version
	})

	return files, nil
}

type migrationFile struct {
	version string
	path    string
}
