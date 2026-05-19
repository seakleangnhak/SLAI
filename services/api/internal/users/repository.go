package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var ErrNotFound = errors.New("user not found")

type Repository struct {
	db platformdb.Executor
}

func NewRepository(db platformdb.Executor) Repository {
	return Repository{db: db}
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (r Repository) Create(ctx context.Context, email, passwordHash, role string) (User, error) {
	email = NormalizeEmail(email)
	if role == "" {
		role = RoleUser
	}

	var user User
	err := r.db.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, status, auth_provider)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, email, COALESCE(password_hash, ''), role, status, auth_provider, COALESCE(google_subject, ''), balance_policy, created_at, updated_at
	`, email, passwordHash, role, StatusActive, AuthProviderPassword).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.AuthProvider,
		&user.GoogleSubject,
		&user.BalancePolicy,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (r Repository) CreateGoogle(ctx context.Context, email, googleSubject, role string) (User, error) {
	email = NormalizeEmail(email)
	googleSubject = strings.TrimSpace(googleSubject)
	if role == "" {
		role = RoleUser
	}

	var user User
	err := r.db.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, status, auth_provider, google_subject)
		VALUES ($1, NULL, $2, $3, $4, $5)
		RETURNING id::text, email, COALESCE(password_hash, ''), role, status, auth_provider, COALESCE(google_subject, ''), balance_policy, created_at, updated_at
	`, email, role, StatusActive, AuthProviderGoogle, googleSubject).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.AuthProvider,
		&user.GoogleSubject,
		&user.BalancePolicy,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return User{}, fmt.Errorf("create google user: %w", err)
	}

	return user, nil
}

func (r Repository) LinkGoogle(ctx context.Context, userID, googleSubject string) (User, error) {
	googleSubject = strings.TrimSpace(googleSubject)
	var user User
	err := r.db.QueryRow(ctx, `
		UPDATE users
		SET auth_provider = CASE WHEN password_hash IS NULL THEN $3 ELSE auth_provider END,
		    google_subject = $2
		WHERE id = $1
		  AND (google_subject IS NULL OR google_subject = $2)
		RETURNING id::text, email, COALESCE(password_hash, ''), role, status, auth_provider, COALESCE(google_subject, ''), balance_policy, created_at, updated_at
	`, userID, googleSubject, AuthProviderGoogle).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.AuthProvider,
		&user.GoogleSubject,
		&user.BalancePolicy,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("link google user: %w", err)
	}
	return user, nil
}

func (r Repository) GetByID(ctx context.Context, id string) (User, error) {
	return r.get(ctx, `WHERE id = $1`, id)
}

func (r Repository) GetByEmail(ctx context.Context, email string) (User, error) {
	return r.get(ctx, `WHERE email = $1`, NormalizeEmail(email))
}

func (r Repository) GetByGoogleSubject(ctx context.Context, googleSubject string) (User, error) {
	return r.get(ctx, `WHERE google_subject = $1`, strings.TrimSpace(googleSubject))
}

func (r Repository) get(ctx context.Context, where string, arg any) (User, error) {
	var user User
	err := r.db.QueryRow(ctx, `
		SELECT id::text, email, COALESCE(password_hash, ''), role, status, auth_provider, COALESCE(google_subject, ''), balance_policy, created_at, updated_at
		FROM users
		`+where,
		arg,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.AuthProvider,
		&user.GoogleSubject,
		&user.BalancePolicy,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}
