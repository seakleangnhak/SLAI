package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/ledger"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	"github.com/slai/slai/services/api/internal/users"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

type Service struct {
	pool     *pgxpool.Pool
	sessions SessionManager
}

func NewService(pool *pgxpool.Pool, sessions SessionManager) Service {
	return Service{pool: pool, sessions: sessions}
}

func (s Service) Signup(ctx context.Context, email, password string) (users.User, string, error) {
	passwordHash, err := HashPassword(password)
	if err != nil {
		return users.User{}, "", err
	}

	var created users.User
	var token string
	err = platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		userRepo := users.NewRepository(tx)
		user, err := userRepo.Create(ctx, email, passwordHash, users.RoleUser)
		if err != nil {
			return err
		}

		ledgerService := ledger.NewService(tx)
		if err := ledgerService.EnsureBalance(ctx, user.ID); err != nil {
			return err
		}

		sessionManager := NewSessionManager(tx, string(s.sessions.secret), s.sessions.cookieSecure, s.sessions.ttl)
		sessionToken, _, err := sessionManager.Create(ctx, user.ID)
		if err != nil {
			return err
		}

		created = user
		token = sessionToken
		return nil
	})
	if err != nil {
		return users.User{}, "", fmt.Errorf("signup: %w", err)
	}

	return created, token, nil
}

func (s Service) Login(ctx context.Context, email, password string) (users.User, string, error) {
	user, err := users.NewRepository(s.pool).GetByEmail(ctx, email)
	if errors.Is(err, users.ErrNotFound) {
		return users.User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return users.User{}, "", err
	}
	if !user.IsActive() {
		return users.User{}, "", ErrInvalidCredentials
	}

	valid, err := VerifyPassword(password, user.PasswordHash)
	if err != nil || !valid {
		return users.User{}, "", ErrInvalidCredentials
	}

	token, _, err := s.sessions.Create(ctx, user.ID)
	if err != nil {
		return users.User{}, "", err
	}

	return user, token, nil
}
