package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	"github.com/slai/slai/services/api/internal/users"
)

const SessionCookieName = "slai_session"

var ErrUnauthenticated = errors.New("unauthenticated")

type SessionManager struct {
	db           platformdb.Executor
	secret       []byte
	cookieSecure bool
	ttl          time.Duration
}

func NewSessionManager(db platformdb.Executor, secret string, cookieSecure bool, ttl time.Duration) SessionManager {
	return SessionManager{
		db:           db,
		secret:       []byte(secret),
		cookieSecure: cookieSecure,
		ttl:          ttl,
	}
}

func (m SessionManager) Create(ctx context.Context, userID string) (string, time.Time, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().UTC().Add(m.ttl)
	_, err = m.db.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, m.hashToken(token), expiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}

	return token, expiresAt, nil
}

func (m SessionManager) Authenticate(ctx context.Context, token string) (users.User, error) {
	if token == "" {
		return users.User{}, ErrUnauthenticated
	}

	var user users.User
	err := m.db.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.password_hash, u.role, u.status, u.balance_policy, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()
	`, m.hashToken(token)).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.BalancePolicy,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return users.User{}, ErrUnauthenticated
	}
	if err != nil {
		return users.User{}, fmt.Errorf("authenticate session: %w", err)
	}
	if !user.IsActive() {
		return users.User{}, ErrUnauthenticated
	}

	return user, nil
}

func (m SessionManager) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	_, err := m.db.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, m.hashToken(token))
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (m SessionManager) SetCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   m.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m SessionManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m SessionManager) hashToken(token string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
