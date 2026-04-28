package apikeys

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/ledger"
	"github.com/slai/slai/services/api/internal/omniroute"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var (
	ErrNotFound            = errors.New("api key not found")
	ErrActiveKeyExists     = errors.New("user already has an active api key")
	ErrInvalidName         = errors.New("api key name is required")
	ErrInsufficientBalance = errors.New("user balance must be greater than zero to resume api key")
	ErrOmniRouteRawKey     = errors.New("omniroute did not return a raw api key")
	ErrRawKeyTooShort      = errors.New("api key is too short to store a safe display prefix")
)

type Config struct {
	Pepper           string
	Prefix           string
	OmniRouteEnabled bool
}

type Service struct {
	pool      *pgxpool.Pool
	cfg       Config
	omniRoute omniroute.Client
	logger    *slog.Logger
}

func NewService(pool *pgxpool.Pool, cfg Config, omniRoute omniroute.Client, logger *slog.Logger) Service {
	if cfg.Pepper == "" {
		cfg.Pepper = "dev-only-change-me-api-key-pepper"
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "sk_slai"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return Service{pool: pool, cfg: cfg, omniRoute: omniRoute, logger: logger}
}

func (s Service) LocalDevMode() bool {
	return !s.cfg.OmniRouteEnabled
}

func (s Service) GetCurrentAPIKey(ctx context.Context, userID string) (APIKey, error) {
	return getCurrentAPIKey(ctx, s.pool, userID)
}

func (s Service) GetLatestAPIKey(ctx context.Context, userID string) (APIKey, error) {
	return getLatestAPIKey(ctx, s.pool, userID)
}

func (s Service) CreateAPIKey(ctx context.Context, userID, name string) (CreatedAPIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CreatedAPIKey{}, ErrInvalidName
	}

	var result CreatedAPIKey
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		email, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		exists, err := activeKeyExists(ctx, tx, userID)
		if err != nil {
			return err
		}
		if exists {
			return ErrActiveKeyExists
		}

		created, rawKey, err := s.createKeyInTx(ctx, tx, userID, email, name)
		if err != nil {
			return err
		}
		result = CreatedAPIKey{APIKey: created.Public(!s.cfg.OmniRouteEnabled), RawAPIKey: rawKey}
		return nil
	})
	if err != nil {
		return CreatedAPIKey{}, err
	}

	s.logger.Info("api key created", "user_id", userID, "key_id", result.APIKey.ID, "omniroute_linked", result.APIKey.OmniRouteLinked)
	return result, nil
}

func (s Service) RotateAPIKey(ctx context.Context, userID string) (CreatedAPIKey, error) {
	var result CreatedAPIKey
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		email, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		current, err := getCurrentAPIKey(ctx, tx, userID)
		if err != nil {
			return err
		}

		if err := s.deleteOmniRouteKey(ctx, current); err != nil {
			return err
		}
		if err := markKeyRevoked(ctx, tx, current.ID); err != nil {
			return err
		}

		created, rawKey, err := s.createKeyInTx(ctx, tx, userID, email, current.Name)
		if err != nil {
			return err
		}
		result = CreatedAPIKey{APIKey: created.Public(!s.cfg.OmniRouteEnabled), RawAPIKey: rawKey}
		return nil
	})
	if err != nil {
		return CreatedAPIKey{}, err
	}

	s.logger.Info("api key rotated", "user_id", userID, "key_id", result.APIKey.ID, "omniroute_linked", result.APIKey.OmniRouteLinked)
	return result, nil
}

func (s Service) RevokeAPIKey(ctx context.Context, userID string) (APIKey, error) {
	var revoked APIKey
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		current, err := getCurrentAPIKey(ctx, tx, userID)
		if err != nil {
			return err
		}
		if err := s.deleteOmniRouteKey(ctx, current); err != nil {
			return err
		}
		updated, err := markKeyRevokedReturning(ctx, tx, current.ID)
		if err != nil {
			return err
		}
		revoked = updated
		return nil
	})
	if err != nil {
		return APIKey{}, err
	}

	s.logger.Info("api key revoked", "user_id", userID, "key_id", revoked.ID)
	return revoked, nil
}

func (s Service) SuspendAPIKey(ctx context.Context, userID string) (APIKey, error) {
	var suspended APIKey
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		current, err := getCurrentAPIKey(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.Status == StatusSuspended {
			suspended = current
			return nil
		}
		isActive := false
		if err := s.updateOmniRouteKey(ctx, current, omniroute.UpdateAPIKeyPayload{IsActive: &isActive}); err != nil {
			return err
		}
		updated, err := updateStatusReturning(ctx, tx, current.ID, StatusSuspended)
		if err != nil {
			return err
		}
		suspended = updated
		return nil
	})
	if err != nil {
		return APIKey{}, err
	}

	s.logger.Info("api key suspended", "user_id", userID, "key_id", suspended.ID)
	return suspended, nil
}

func (s Service) ResumeAPIKey(ctx context.Context, userID string) (APIKey, error) {
	var resumed APIKey
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		balance, err := ledger.NewService(tx).GetBalance(ctx, userID)
		if err != nil {
			return err
		}
		if balance.AvailableUnits <= 0 {
			return ErrInsufficientBalance
		}

		current, err := getCurrentAPIKey(ctx, tx, userID)
		if err != nil {
			return err
		}
		if current.Status == StatusActive {
			resumed = current
			return nil
		}

		isActive := true
		if err := s.updateOmniRouteKey(ctx, current, omniroute.UpdateAPIKeyPayload{IsActive: &isActive}); err != nil {
			return err
		}
		updated, err := updateStatusReturning(ctx, tx, current.ID, StatusActive)
		if err != nil {
			return err
		}
		resumed = updated
		return nil
	})
	if err != nil {
		return APIKey{}, err
	}

	s.logger.Info("api key resumed", "user_id", userID, "key_id", resumed.ID)
	return resumed, nil
}

func (s Service) createKeyInTx(ctx context.Context, tx pgx.Tx, userID, email, name string) (APIKey, string, error) {
	rawKey := ""
	var omniRouteKeyID *string

	if s.cfg.OmniRouteEnabled {
		if s.omniRoute == nil {
			return APIKey{}, "", errors.New("omniroute client is required when OMNIROUTE_ENABLED=true")
		}
		omniKey, err := s.omniRoute.CreateAPIKey(ctx, "SLAI "+email)
		if err != nil {
			return APIKey{}, "", fmt.Errorf("create omniroute api key: %w", err)
		}
		if omniKey.RawKey == "" {
			return APIKey{}, "", ErrOmniRouteRawKey
		}
		rawKey = omniKey.RawKey
		if omniKey.ID != "" {
			omniRouteKeyID = &omniKey.ID
		}
	} else {
		generated, err := GenerateRawKey(s.cfg.Prefix)
		if err != nil {
			return APIKey{}, "", err
		}
		rawKey = generated
	}
	if len(rawKey) <= displayPrefixLength {
		return APIKey{}, "", ErrRawKeyTooShort
	}

	key, err := insertAPIKey(ctx, tx, APIKey{
		UserID:         userID,
		OmniRouteKeyID: omniRouteKeyID,
		KeyHash:        HashRawKey(rawKey, s.cfg.Pepper),
		KeyPrefix:      DisplayPrefix(rawKey),
		Name:           name,
		Status:         StatusActive,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return APIKey{}, "", ErrActiveKeyExists
		}
		return APIKey{}, "", err
	}

	return key, rawKey, nil
}

func (s Service) deleteOmniRouteKey(ctx context.Context, key APIKey) error {
	if !s.cfg.OmniRouteEnabled || key.OmniRouteKeyID == nil || *key.OmniRouteKeyID == "" {
		return nil
	}
	if s.omniRoute == nil {
		return errors.New("omniroute client is required when OMNIROUTE_ENABLED=true")
	}
	if err := s.omniRoute.DeleteAPIKey(ctx, *key.OmniRouteKeyID); err != nil {
		return fmt.Errorf("delete omniroute api key: %w", err)
	}
	return nil
}

func (s Service) updateOmniRouteKey(ctx context.Context, key APIKey, payload omniroute.UpdateAPIKeyPayload) error {
	if !s.cfg.OmniRouteEnabled || key.OmniRouteKeyID == nil || *key.OmniRouteKeyID == "" {
		return nil
	}
	if s.omniRoute == nil {
		return errors.New("omniroute client is required when OMNIROUTE_ENABLED=true")
	}
	if err := s.omniRoute.UpdateAPIKey(ctx, *key.OmniRouteKeyID, payload); err != nil {
		return fmt.Errorf("update omniroute api key: %w", err)
	}
	return nil
}

func lockUser(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var email string
	err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("user not found")
	}
	if err != nil {
		return "", fmt.Errorf("lock user: %w", err)
	}
	return email, nil
}

func activeKeyExists(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM api_keys WHERE user_id = $1 AND status = $2)`, userID, StatusActive).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check active api key: %w", err)
	}
	return exists, nil
}

func getCurrentAPIKey(ctx context.Context, db platformdb.Executor, userID string) (APIKey, error) {
	return scanAPIKey(db.QueryRow(ctx, `
		SELECT id::text, user_id::text, omniroute_key_id, key_hash, key_prefix, name, status, last_used_at, revoked_at, created_at, updated_at
		FROM api_keys
		WHERE user_id = $1 AND status IN ($2, $3)
		ORDER BY created_at DESC
		LIMIT 1
	`, userID, StatusActive, StatusSuspended))
}

func getLatestAPIKey(ctx context.Context, db platformdb.Executor, userID string) (APIKey, error) {
	return scanAPIKey(db.QueryRow(ctx, `
		SELECT id::text, user_id::text, omniroute_key_id, key_hash, key_prefix, name, status, last_used_at, revoked_at, created_at, updated_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, userID))
}

func insertAPIKey(ctx context.Context, tx pgx.Tx, key APIKey) (APIKey, error) {
	return scanAPIKey(tx.QueryRow(ctx, `
		INSERT INTO api_keys (user_id, omniroute_key_id, key_hash, key_prefix, name, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, user_id::text, omniroute_key_id, key_hash, key_prefix, name, status, last_used_at, revoked_at, created_at, updated_at
	`, key.UserID, key.OmniRouteKeyID, key.KeyHash, key.KeyPrefix, key.Name, key.Status))
}

func markKeyRevoked(ctx context.Context, tx pgx.Tx, keyID string) error {
	_, err := tx.Exec(ctx, `UPDATE api_keys SET status = $2, revoked_at = now() WHERE id = $1`, keyID, StatusRevoked)
	if err != nil {
		return fmt.Errorf("mark api key revoked: %w", err)
	}
	return nil
}

func markKeyRevokedReturning(ctx context.Context, tx pgx.Tx, keyID string) (APIKey, error) {
	return scanAPIKey(tx.QueryRow(ctx, `
		UPDATE api_keys
		SET status = $2, revoked_at = COALESCE(revoked_at, now())
		WHERE id = $1
		RETURNING id::text, user_id::text, omniroute_key_id, key_hash, key_prefix, name, status, last_used_at, revoked_at, created_at, updated_at
	`, keyID, StatusRevoked))
}

func updateStatusReturning(ctx context.Context, tx pgx.Tx, keyID, status string) (APIKey, error) {
	return scanAPIKey(tx.QueryRow(ctx, `
		UPDATE api_keys
		SET status = $2
		WHERE id = $1
		RETURNING id::text, user_id::text, omniroute_key_id, key_hash, key_prefix, name, status, last_used_at, revoked_at, created_at, updated_at
	`, keyID, status))
}

func scanAPIKey(row pgx.Row) (APIKey, error) {
	var key APIKey
	err := row.Scan(
		&key.ID,
		&key.UserID,
		&key.OmniRouteKeyID,
		&key.KeyHash,
		&key.KeyPrefix,
		&key.Name,
		&key.Status,
		&key.LastUsedAt,
		&key.RevokedAt,
		&key.CreatedAt,
		&key.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	if err != nil {
		return APIKey{}, fmt.Errorf("scan api key: %w", err)
	}
	return key, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
