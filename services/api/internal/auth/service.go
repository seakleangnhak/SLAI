package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/ledger"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	"github.com/slai/slai/services/api/internal/users"
)

const (
	defaultSignupOTPTTL                      = 10 * time.Minute
	defaultSignupOTPResendCooldown           = time.Minute
	defaultSignupOTPRequestWindow            = time.Hour
	defaultSignupOTPMaxEmailRequests         = 5
	defaultSignupOTPMaxIPRequests            = 20
	defaultPasswordResetOTPTTL               = 10 * time.Minute
	defaultPasswordResetOTPResendCooldown    = time.Minute
	defaultPasswordResetOTPRequestWindow     = time.Hour
	defaultPasswordResetOTPMaxEmailRequests  = 5
	defaultPasswordResetOTPMaxIPRequests     = 20
	maxSignupOTPAttempts                     = 5
	maxPasswordResetOTPAttempts              = 5
	otpCodeUpperBound                        = 1000000
	signupOTPHashMessagePart                 = "signup-email-verification"
	signupOTPRateLimitHashMessagePart        = "signup-otp-rate-limit"
	passwordResetOTPHashMessagePart          = "password-reset"
	passwordResetOTPRateLimitHashMessagePart = "password-reset-otp-rate-limit"
)

var (
	ErrInvalidCredentials      = errors.New("invalid email or password")
	ErrEmailAlreadyRegistered  = errors.New("email is already registered")
	ErrEmailDeliveryFailed     = errors.New("email delivery failed")
	ErrInvalidCurrentPassword  = errors.New("current password is invalid")
	ErrPasswordAuthUnavailable = errors.New("password authentication is not available")
	ErrInvalidVerificationCode = errors.New("invalid verification code")
	ErrVerificationCodeExpired = errors.New("verification code expired")
	ErrTooManyOTPAttempts      = errors.New("too many verification attempts")
	ErrTooManyOTPRequests      = errors.New("too many verification code requests")
	ErrOTPResendTooSoon        = errors.New("verification code requested too soon")
)

type OTPRateLimitError struct {
	Err        error
	RetryAfter time.Duration
}

func (e OTPRateLimitError) Error() string {
	if e.Err == nil {
		return "OTP rate limited"
	}
	return e.Err.Error()
}

func (e OTPRateLimitError) Unwrap() error {
	return e.Err
}

type SignupOTPResult struct {
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type PasswordResetOTPResult struct {
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Service struct {
	pool                             *pgxpool.Pool
	sessions                         SessionManager
	googleVerifier                   GoogleIdentityVerifier
	emailSender                      EmailSender
	signupOTPTTL                     time.Duration
	signupOTPResendCooldown          time.Duration
	signupOTPRequestWindow           time.Duration
	signupOTPMaxEmailRequests        int
	signupOTPMaxIPRequests           int
	passwordResetOTPTTL              time.Duration
	passwordResetOTPResendCooldown   time.Duration
	passwordResetOTPRequestWindow    time.Duration
	passwordResetOTPMaxEmailRequests int
	passwordResetOTPMaxIPRequests    int
	passwordChangedAlertsEnabled     bool
}

func NewService(pool *pgxpool.Pool, sessions SessionManager) Service {
	return Service{
		pool:                             pool,
		sessions:                         sessions,
		emailSender:                      NoopEmailSender{},
		signupOTPTTL:                     defaultSignupOTPTTL,
		signupOTPResendCooldown:          defaultSignupOTPResendCooldown,
		signupOTPRequestWindow:           defaultSignupOTPRequestWindow,
		signupOTPMaxEmailRequests:        defaultSignupOTPMaxEmailRequests,
		signupOTPMaxIPRequests:           defaultSignupOTPMaxIPRequests,
		passwordResetOTPTTL:              defaultPasswordResetOTPTTL,
		passwordResetOTPResendCooldown:   defaultPasswordResetOTPResendCooldown,
		passwordResetOTPRequestWindow:    defaultPasswordResetOTPRequestWindow,
		passwordResetOTPMaxEmailRequests: defaultPasswordResetOTPMaxEmailRequests,
		passwordResetOTPMaxIPRequests:    defaultPasswordResetOTPMaxIPRequests,
		passwordChangedAlertsEnabled:     true,
	}
}

func (s Service) WithGoogleVerifier(verifier GoogleIdentityVerifier) Service {
	s.googleVerifier = verifier
	return s
}

func (s Service) WithEmailSender(sender EmailSender) Service {
	if sender != nil {
		s.emailSender = sender
	}
	return s
}

func (s Service) WithSignupOTPTTL(ttl time.Duration) Service {
	if ttl > 0 {
		s.signupOTPTTL = ttl
	}
	return s
}

func (s Service) WithSignupOTPControls(resendCooldown, requestWindow time.Duration, maxEmailRequests, maxIPRequests int) Service {
	if resendCooldown > 0 {
		s.signupOTPResendCooldown = resendCooldown
	}
	if requestWindow > 0 {
		s.signupOTPRequestWindow = requestWindow
	}
	if maxEmailRequests > 0 {
		s.signupOTPMaxEmailRequests = maxEmailRequests
	}
	if maxIPRequests > 0 {
		s.signupOTPMaxIPRequests = maxIPRequests
	}
	return s
}

func (s Service) WithPasswordResetOTPTTL(ttl time.Duration) Service {
	if ttl > 0 {
		s.passwordResetOTPTTL = ttl
	}
	return s
}

func (s Service) WithPasswordResetOTPControls(resendCooldown, requestWindow time.Duration, maxEmailRequests, maxIPRequests int) Service {
	if resendCooldown > 0 {
		s.passwordResetOTPResendCooldown = resendCooldown
	}
	if requestWindow > 0 {
		s.passwordResetOTPRequestWindow = requestWindow
	}
	if maxEmailRequests > 0 {
		s.passwordResetOTPMaxEmailRequests = maxEmailRequests
	}
	if maxIPRequests > 0 {
		s.passwordResetOTPMaxIPRequests = maxIPRequests
	}
	return s
}

func (s Service) WithPasswordChangedAlerts(enabled bool) Service {
	s.passwordChangedAlertsEnabled = enabled
	return s
}

func (s Service) Signup(ctx context.Context, email, password, clientIP string) (SignupOTPResult, error) {
	email = users.NormalizeEmail(email)
	if email == "" {
		return SignupOTPResult{}, errors.New("email is required")
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return SignupOTPResult{}, err
	}

	otp, err := generateOTP()
	if err != nil {
		return SignupOTPResult{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(s.signupOTPTTL)
	otpHash := s.hashSignupOTP(email, otp)

	err = platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.cleanupExpiredSignupOTPState(ctx, tx, now); err != nil {
			return err
		}

		userRepo := users.NewRepository(tx)
		_, err := userRepo.GetByEmail(ctx, email)
		if err == nil {
			return ErrEmailAlreadyRegistered
		}
		if !errors.Is(err, users.ErrNotFound) {
			return err
		}

		requestCount, firstRequestedAt, err := s.nextSignupOTPEmailRequest(ctx, tx, email, now)
		if err != nil {
			return err
		}
		if err := s.checkSignupOTPEmailRateLimit(ctx, tx, email, now); err != nil {
			return err
		}
		if err := s.checkSignupOTPIPRateLimit(ctx, tx, clientIP, now); err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO signup_email_verifications (
			    email,
			    password_hash,
			    otp_hash,
			    attempts,
			    expires_at,
			    request_count,
			    first_requested_at,
			    last_sent_at
			)
			VALUES ($1, $2, $3, 0, $4, $5, $6, $7)
			ON CONFLICT (email) DO UPDATE
			SET password_hash = EXCLUDED.password_hash,
			    otp_hash = EXCLUDED.otp_hash,
			    attempts = 0,
			    expires_at = EXCLUDED.expires_at,
			    request_count = EXCLUDED.request_count,
			    first_requested_at = EXCLUDED.first_requested_at,
			    last_sent_at = EXCLUDED.last_sent_at
		`, email, passwordHash, otpHash, expiresAt, requestCount, firstRequestedAt, now)
		if err != nil {
			return fmt.Errorf("store signup OTP: %w", err)
		}
		return nil
	})
	if err != nil {
		return SignupOTPResult{}, fmt.Errorf("signup: %w", err)
	}

	if err := s.emailSender.SendSignupOTP(ctx, email, otp, expiresAt); err != nil {
		return SignupOTPResult{}, fmt.Errorf("%w: %v", ErrEmailDeliveryFailed, err)
	}

	return SignupOTPResult{Email: email, ExpiresAt: expiresAt}, nil
}

func (s Service) VerifySignupEmailOTP(ctx context.Context, email, otp string) (users.User, string, error) {
	email = users.NormalizeEmail(email)
	otp = strings.TrimSpace(otp)
	if email == "" || otp == "" {
		return users.User{}, "", ErrInvalidVerificationCode
	}

	var created users.User
	var token string
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var passwordHash string
		var otpHash string
		var attempts int
		var expiresAt time.Time
		err := tx.QueryRow(ctx, `
			SELECT password_hash, otp_hash, attempts, expires_at
			FROM signup_email_verifications
			WHERE email = $1
			FOR UPDATE
		`, email).Scan(&passwordHash, &otpHash, &attempts, &expiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidVerificationCode
		}
		if err != nil {
			return fmt.Errorf("get signup OTP: %w", err)
		}
		if !time.Now().UTC().Before(expiresAt) {
			_, _ = tx.Exec(ctx, `DELETE FROM signup_email_verifications WHERE email = $1`, email)
			return ErrVerificationCodeExpired
		}
		if attempts >= maxSignupOTPAttempts {
			_, _ = tx.Exec(ctx, `DELETE FROM signup_email_verifications WHERE email = $1`, email)
			return ErrTooManyOTPAttempts
		}
		if !s.verifySignupOTPHash(email, otp, otpHash) {
			_, _ = tx.Exec(ctx, `
				UPDATE signup_email_verifications
				SET attempts = attempts + 1
				WHERE email = $1
			`, email)
			return ErrInvalidVerificationCode
		}

		userRepo := users.NewRepository(tx)
		if _, err := userRepo.GetByEmail(ctx, email); err == nil {
			_, _ = tx.Exec(ctx, `DELETE FROM signup_email_verifications WHERE email = $1`, email)
			return ErrEmailAlreadyRegistered
		} else if !errors.Is(err, users.ErrNotFound) {
			return err
		}

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

		if _, err := tx.Exec(ctx, `DELETE FROM signup_email_verifications WHERE email = $1`, email); err != nil {
			return fmt.Errorf("clear signup OTP: %w", err)
		}

		created = user
		token = sessionToken
		return nil
	})
	if err != nil {
		return users.User{}, "", fmt.Errorf("verify signup email OTP: %w", err)
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
	if user.PasswordHash == "" {
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

func (s Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	currentPassword = strings.TrimSpace(currentPassword)
	if currentPassword == "" {
		return ErrInvalidCurrentPassword
	}

	newPasswordHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	var email string
	changedAt := time.Now().UTC()
	err = platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var user users.User
		err := tx.QueryRow(ctx, `
			SELECT id::text, email, COALESCE(password_hash, ''), role, status, auth_provider, COALESCE(google_subject, ''), balance_policy, created_at, updated_at
			FROM users
			WHERE id = $1
			FOR UPDATE
		`, userID).Scan(
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
			return ErrInvalidCredentials
		}
		if err != nil {
			return fmt.Errorf("get user for password change: %w", err)
		}
		if !user.IsActive() {
			return ErrInvalidCredentials
		}
		if user.PasswordHash == "" {
			return ErrPasswordAuthUnavailable
		}

		valid, err := VerifyPassword(currentPassword, user.PasswordHash)
		if err != nil || !valid {
			return ErrInvalidCurrentPassword
		}

		if _, err := tx.Exec(ctx, `
			UPDATE users
			SET password_hash = $2,
			    auth_provider = $3
			WHERE id = $1
		`, user.ID, newPasswordHash, users.AuthProviderPassword); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE sessions
			SET revoked_at = now()
			WHERE user_id = $1 AND revoked_at IS NULL
		`, user.ID); err != nil {
			return fmt.Errorf("revoke sessions after password change: %w", err)
		}
		email = user.Email
		return nil
	})
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	if s.passwordChangedAlertsEnabled && email != "" {
		_ = s.emailSender.SendPasswordChangedAlert(ctx, email, changedAt)
	}
	return nil
}

func (s Service) RequestPasswordReset(ctx context.Context, email, clientIP string) (PasswordResetOTPResult, error) {
	email = users.NormalizeEmail(email)
	if email == "" {
		return PasswordResetOTPResult{}, errors.New("email is required")
	}

	otp, err := generateOTP()
	if err != nil {
		return PasswordResetOTPResult{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(s.passwordResetOTPTTL)
	otpHash := s.hashPasswordResetOTP(email, otp)
	shouldSend := false

	err = platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.cleanupExpiredPasswordResetOTPState(ctx, tx, now); err != nil {
			return err
		}

		requestCount, firstRequestedAt, err := s.nextPasswordResetOTPEmailRequest(ctx, tx, email, now)
		if err != nil {
			return err
		}
		if err := s.checkPasswordResetOTPEmailRateLimit(ctx, tx, email, now); err != nil {
			return err
		}
		if err := s.checkPasswordResetOTPIPRateLimit(ctx, tx, clientIP, now); err != nil {
			return err
		}

		user, err := users.NewRepository(tx).GetByEmail(ctx, email)
		if err != nil && !errors.Is(err, users.ErrNotFound) {
			return err
		}
		shouldSend = err == nil && user.IsActive() && user.PasswordHash != ""

		_, err = tx.Exec(ctx, `
			INSERT INTO password_reset_otps (
			    email,
			    otp_hash,
			    attempts,
			    request_count,
			    first_requested_at,
			    last_sent_at,
			    expires_at
			)
			VALUES ($1, $2, 0, $3, $4, $5, $6)
			ON CONFLICT (email) DO UPDATE
			SET otp_hash = EXCLUDED.otp_hash,
			    attempts = 0,
			    request_count = EXCLUDED.request_count,
			    first_requested_at = EXCLUDED.first_requested_at,
			    last_sent_at = EXCLUDED.last_sent_at,
			    expires_at = EXCLUDED.expires_at
		`, email, otpHash, requestCount, firstRequestedAt, now, expiresAt)
		if err != nil {
			return fmt.Errorf("store password reset OTP: %w", err)
		}
		return nil
	})
	if err != nil {
		return PasswordResetOTPResult{}, fmt.Errorf("request password reset: %w", err)
	}

	if shouldSend {
		if err := s.emailSender.SendPasswordResetOTP(ctx, email, otp, expiresAt); err != nil {
			return PasswordResetOTPResult{}, fmt.Errorf("%w: %v", ErrEmailDeliveryFailed, err)
		}
	}

	return PasswordResetOTPResult{Email: email, ExpiresAt: expiresAt}, nil
}

func (s Service) ResetPassword(ctx context.Context, email, otp, password string) error {
	email = users.NormalizeEmail(email)
	otp = strings.TrimSpace(otp)
	if email == "" || otp == "" {
		return ErrInvalidVerificationCode
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return err
	}

	changedAt := time.Now().UTC()
	err = platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var otpHash string
		var attempts int
		var expiresAt time.Time
		err := tx.QueryRow(ctx, `
			SELECT otp_hash, attempts, expires_at
			FROM password_reset_otps
			WHERE email = $1
			FOR UPDATE
		`, email).Scan(&otpHash, &attempts, &expiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidVerificationCode
		}
		if err != nil {
			return fmt.Errorf("get password reset OTP: %w", err)
		}
		if !time.Now().UTC().Before(expiresAt) {
			_, _ = tx.Exec(ctx, `DELETE FROM password_reset_otps WHERE email = $1`, email)
			return ErrVerificationCodeExpired
		}
		if attempts >= maxPasswordResetOTPAttempts {
			_, _ = tx.Exec(ctx, `DELETE FROM password_reset_otps WHERE email = $1`, email)
			return ErrTooManyOTPAttempts
		}
		if !s.verifyPasswordResetOTPHash(email, otp, otpHash) {
			_, _ = tx.Exec(ctx, `
				UPDATE password_reset_otps
				SET attempts = attempts + 1
				WHERE email = $1
			`, email)
			return ErrInvalidVerificationCode
		}

		var user users.User
		err = tx.QueryRow(ctx, `
			UPDATE users
			SET password_hash = $2,
			    auth_provider = $3
			WHERE email = $1
			  AND password_hash IS NOT NULL
			  AND status = $4
			RETURNING id::text, email, COALESCE(password_hash, ''), role, status, auth_provider, COALESCE(google_subject, ''), balance_policy, created_at, updated_at
		`, email, passwordHash, users.AuthProviderPassword, users.StatusActive).Scan(
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
			_, _ = tx.Exec(ctx, `DELETE FROM password_reset_otps WHERE email = $1`, email)
			return ErrInvalidVerificationCode
		}
		if err != nil {
			return fmt.Errorf("update password: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE sessions
			SET revoked_at = now()
			WHERE user_id = $1 AND revoked_at IS NULL
		`, user.ID); err != nil {
			return fmt.Errorf("revoke sessions after password reset: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM password_reset_otps WHERE email = $1`, email); err != nil {
			return fmt.Errorf("clear password reset OTP: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	if s.passwordChangedAlertsEnabled {
		_ = s.emailSender.SendPasswordChangedAlert(ctx, email, changedAt)
	}
	return nil
}

func (s Service) GoogleLogin(ctx context.Context, credential string) (users.User, string, error) {
	if s.googleVerifier == nil {
		return users.User{}, "", ErrGoogleAuthDisabled
	}

	identity, err := s.googleVerifier.VerifyGoogleIDToken(ctx, credential)
	if err != nil {
		return users.User{}, "", err
	}

	var authenticated users.User
	var token string
	err = platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		userRepo := users.NewRepository(tx)

		user, err := userRepo.GetByGoogleSubject(ctx, identity.Subject)
		if errors.Is(err, users.ErrNotFound) {
			user, err = s.userForGoogleEmail(ctx, userRepo, identity)
		}
		if err != nil {
			return err
		}
		if !user.IsActive() {
			return ErrInvalidCredentials
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

		authenticated = user
		token = sessionToken
		return nil
	})
	if err != nil {
		return users.User{}, "", fmt.Errorf("google login: %w", err)
	}

	return authenticated, token, nil
}

func (s Service) userForGoogleEmail(ctx context.Context, userRepo users.Repository, identity GoogleIdentity) (users.User, error) {
	user, err := userRepo.GetByEmail(ctx, identity.Email)
	if errors.Is(err, users.ErrNotFound) {
		return userRepo.CreateGoogle(ctx, identity.Email, identity.Subject, users.RoleUser)
	}
	if err != nil {
		return users.User{}, err
	}
	if user.GoogleSubject == "" {
		return userRepo.LinkGoogle(ctx, user.ID, identity.Subject)
	}
	if user.GoogleSubject != identity.Subject {
		return users.User{}, ErrInvalidGoogleToken
	}
	return user, nil
}

func (s Service) hashSignupOTP(email, otp string) string {
	mac := hmac.New(sha256.New, s.sessions.secret)
	_, _ = mac.Write([]byte(signupOTPHashMessagePart))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(email))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(otp))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s Service) verifySignupOTPHash(email, otp, expectedHash string) bool {
	actual := s.hashSignupOTP(email, otp)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func (s Service) hashPasswordResetOTP(email, otp string) string {
	mac := hmac.New(sha256.New, s.sessions.secret)
	_, _ = mac.Write([]byte(passwordResetOTPHashMessagePart))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(email))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(otp))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s Service) verifyPasswordResetOTPHash(email, otp, expectedHash string) bool {
	actual := s.hashPasswordResetOTP(email, otp)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func (s Service) CleanupExpiredSignupOTPState(ctx context.Context) error {
	now := time.Now().UTC()
	return platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.cleanupExpiredSignupOTPState(ctx, tx, now); err != nil {
			return err
		}
		return s.cleanupExpiredPasswordResetOTPState(ctx, tx, now)
	})
}

func (s Service) CleanupExpiredPasswordResetOTPState(ctx context.Context) error {
	now := time.Now().UTC()
	return platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		return s.cleanupExpiredPasswordResetOTPState(ctx, tx, now)
	})
}

func (s Service) nextSignupOTPEmailRequest(ctx context.Context, tx pgx.Tx, email string, now time.Time) (int, time.Time, error) {
	var requestCount int
	var firstRequestedAt time.Time
	var lastSentAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT request_count, first_requested_at, last_sent_at
		FROM signup_email_verifications
		WHERE email = $1
		FOR UPDATE
	`, email).Scan(&requestCount, &firstRequestedAt, &lastSentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 1, now, nil
	}
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("get signup OTP request state: %w", err)
	}

	if lastSentAt != nil {
		cooldownEndsAt := lastSentAt.Add(s.signupOTPResendCooldown)
		if now.Before(cooldownEndsAt) {
			return 0, time.Time{}, OTPRateLimitError{Err: ErrOTPResendTooSoon, RetryAfter: cooldownEndsAt.Sub(now)}
		}
	}

	windowEndsAt := firstRequestedAt.Add(s.signupOTPRequestWindow)
	if !now.Before(windowEndsAt) {
		return 1, now, nil
	}
	if requestCount >= s.signupOTPMaxEmailRequests {
		return 0, time.Time{}, OTPRateLimitError{Err: ErrTooManyOTPRequests, RetryAfter: windowEndsAt.Sub(now)}
	}

	return requestCount + 1, firstRequestedAt, nil
}

func (s Service) checkSignupOTPIPRateLimit(ctx context.Context, tx pgx.Tx, clientIP string, now time.Time) error {
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" || s.signupOTPMaxIPRequests <= 0 {
		return nil
	}
	return s.incrementSignupOTPRateLimit(ctx, tx, "ip", s.hashSignupOTPRateLimitKey("ip", clientIP), s.signupOTPMaxIPRequests, now)
}

func (s Service) checkSignupOTPEmailRateLimit(ctx context.Context, tx pgx.Tx, email string, now time.Time) error {
	email = users.NormalizeEmail(email)
	if email == "" || s.signupOTPMaxEmailRequests <= 0 {
		return nil
	}
	return s.incrementSignupOTPRateLimit(ctx, tx, "email", s.hashSignupOTPRateLimitKey("email", email), s.signupOTPMaxEmailRequests, now)
}

func (s Service) incrementSignupOTPRateLimit(ctx context.Context, tx pgx.Tx, scope, keyHash string, maxRequests int, now time.Time) error {
	var requestCount int
	var windowStart time.Time
	err := tx.QueryRow(ctx, `
		SELECT request_count, window_start
		FROM signup_otp_rate_limits
		WHERE scope = $1 AND key_hash = $2
		FOR UPDATE
	`, scope, keyHash).Scan(&requestCount, &windowStart)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `
			INSERT INTO signup_otp_rate_limits (scope, key_hash, request_count, window_start, last_request_at)
			VALUES ($1, $2, 1, $3, $3)
		`, scope, keyHash, now)
		if err != nil {
			return fmt.Errorf("create signup OTP rate limit: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get signup OTP rate limit: %w", err)
	}

	windowEndsAt := windowStart.Add(s.signupOTPRequestWindow)
	if !now.Before(windowEndsAt) {
		_, err = tx.Exec(ctx, `
			UPDATE signup_otp_rate_limits
			SET request_count = 1,
			    window_start = $3,
			    last_request_at = $3
			WHERE scope = $1 AND key_hash = $2
		`, scope, keyHash, now)
		if err != nil {
			return fmt.Errorf("reset signup OTP rate limit: %w", err)
		}
		return nil
	}
	if requestCount >= maxRequests {
		return OTPRateLimitError{Err: ErrTooManyOTPRequests, RetryAfter: windowEndsAt.Sub(now)}
	}

	_, err = tx.Exec(ctx, `
		UPDATE signup_otp_rate_limits
		SET request_count = request_count + 1,
		    last_request_at = $3
		WHERE scope = $1 AND key_hash = $2
	`, scope, keyHash, now)
	if err != nil {
		return fmt.Errorf("update signup OTP rate limit: %w", err)
	}
	return nil
}

func (s Service) cleanupExpiredSignupOTPState(ctx context.Context, tx pgx.Tx, now time.Time) error {
	if _, err := tx.Exec(ctx, `DELETE FROM signup_email_verifications WHERE expires_at < $1`, now); err != nil {
		return fmt.Errorf("cleanup expired signup OTPs: %w", err)
	}
	cutoff := now.Add(-2 * s.signupOTPRequestWindow)
	if _, err := tx.Exec(ctx, `DELETE FROM signup_otp_rate_limits WHERE window_start < $1`, cutoff); err != nil {
		return fmt.Errorf("cleanup signup OTP rate limits: %w", err)
	}
	return nil
}

func (s Service) hashSignupOTPRateLimitKey(scope, value string) string {
	mac := hmac.New(sha256.New, s.sessions.secret)
	_, _ = mac.Write([]byte(signupOTPRateLimitHashMessagePart))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s Service) nextPasswordResetOTPEmailRequest(ctx context.Context, tx pgx.Tx, email string, now time.Time) (int, time.Time, error) {
	var requestCount int
	var firstRequestedAt time.Time
	var lastSentAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT request_count, first_requested_at, last_sent_at
		FROM password_reset_otps
		WHERE email = $1
		FOR UPDATE
	`, email).Scan(&requestCount, &firstRequestedAt, &lastSentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 1, now, nil
	}
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("get password reset OTP request state: %w", err)
	}

	if lastSentAt != nil {
		cooldownEndsAt := lastSentAt.Add(s.passwordResetOTPResendCooldown)
		if now.Before(cooldownEndsAt) {
			return 0, time.Time{}, OTPRateLimitError{Err: ErrOTPResendTooSoon, RetryAfter: cooldownEndsAt.Sub(now)}
		}
	}

	windowEndsAt := firstRequestedAt.Add(s.passwordResetOTPRequestWindow)
	if !now.Before(windowEndsAt) {
		return 1, now, nil
	}
	if requestCount >= s.passwordResetOTPMaxEmailRequests {
		return 0, time.Time{}, OTPRateLimitError{Err: ErrTooManyOTPRequests, RetryAfter: windowEndsAt.Sub(now)}
	}

	return requestCount + 1, firstRequestedAt, nil
}

func (s Service) checkPasswordResetOTPIPRateLimit(ctx context.Context, tx pgx.Tx, clientIP string, now time.Time) error {
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" || s.passwordResetOTPMaxIPRequests <= 0 {
		return nil
	}
	return s.incrementPasswordResetOTPRateLimit(ctx, tx, "ip", s.hashPasswordResetOTPRateLimitKey("ip", clientIP), s.passwordResetOTPMaxIPRequests, now)
}

func (s Service) checkPasswordResetOTPEmailRateLimit(ctx context.Context, tx pgx.Tx, email string, now time.Time) error {
	email = users.NormalizeEmail(email)
	if email == "" || s.passwordResetOTPMaxEmailRequests <= 0 {
		return nil
	}
	return s.incrementPasswordResetOTPRateLimit(ctx, tx, "email", s.hashPasswordResetOTPRateLimitKey("email", email), s.passwordResetOTPMaxEmailRequests, now)
}

func (s Service) incrementPasswordResetOTPRateLimit(ctx context.Context, tx pgx.Tx, scope, keyHash string, maxRequests int, now time.Time) error {
	var requestCount int
	var windowStart time.Time
	err := tx.QueryRow(ctx, `
		SELECT request_count, window_start
		FROM password_reset_otp_rate_limits
		WHERE scope = $1 AND key_hash = $2
		FOR UPDATE
	`, scope, keyHash).Scan(&requestCount, &windowStart)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `
			INSERT INTO password_reset_otp_rate_limits (scope, key_hash, request_count, window_start, last_request_at)
			VALUES ($1, $2, 1, $3, $3)
		`, scope, keyHash, now)
		if err != nil {
			return fmt.Errorf("create password reset OTP rate limit: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get password reset OTP rate limit: %w", err)
	}

	windowEndsAt := windowStart.Add(s.passwordResetOTPRequestWindow)
	if !now.Before(windowEndsAt) {
		_, err = tx.Exec(ctx, `
			UPDATE password_reset_otp_rate_limits
			SET request_count = 1,
			    window_start = $3,
			    last_request_at = $3
			WHERE scope = $1 AND key_hash = $2
		`, scope, keyHash, now)
		if err != nil {
			return fmt.Errorf("reset password reset OTP rate limit: %w", err)
		}
		return nil
	}
	if requestCount >= maxRequests {
		return OTPRateLimitError{Err: ErrTooManyOTPRequests, RetryAfter: windowEndsAt.Sub(now)}
	}

	_, err = tx.Exec(ctx, `
		UPDATE password_reset_otp_rate_limits
		SET request_count = request_count + 1,
		    last_request_at = $3
		WHERE scope = $1 AND key_hash = $2
	`, scope, keyHash, now)
	if err != nil {
		return fmt.Errorf("update password reset OTP rate limit: %w", err)
	}
	return nil
}

func (s Service) cleanupExpiredPasswordResetOTPState(ctx context.Context, tx pgx.Tx, now time.Time) error {
	if _, err := tx.Exec(ctx, `DELETE FROM password_reset_otps WHERE expires_at < $1`, now); err != nil {
		return fmt.Errorf("cleanup expired password reset OTPs: %w", err)
	}
	cutoff := now.Add(-2 * s.passwordResetOTPRequestWindow)
	if _, err := tx.Exec(ctx, `DELETE FROM password_reset_otp_rate_limits WHERE window_start < $1`, cutoff); err != nil {
		return fmt.Errorf("cleanup password reset OTP rate limits: %w", err)
	}
	return nil
}

func (s Service) hashPasswordResetOTPRateLimitKey(scope, value string) string {
	mac := hmac.New(sha256.New, s.sessions.secret)
	_, _ = mac.Write([]byte(passwordResetOTPRateLimitHashMessagePart))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte("\x00"))
	_, _ = mac.Write([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(mac.Sum(nil))
}

func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(otpCodeUpperBound))
	if err != nil {
		return "", fmt.Errorf("generate OTP: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
