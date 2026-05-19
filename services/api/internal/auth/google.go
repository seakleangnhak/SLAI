package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/idtoken"
)

var (
	ErrGoogleAuthDisabled    = errors.New("google auth is not configured")
	ErrInvalidGoogleToken    = errors.New("invalid google credential")
	ErrGoogleEmailUnverified = errors.New("google email is not verified")
)

type GoogleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
}

type GoogleIdentityVerifier interface {
	VerifyGoogleIDToken(ctx context.Context, credential string) (GoogleIdentity, error)
}

type GoogleIDTokenVerifier struct {
	clientID string
}

func NewGoogleIDTokenVerifier(clientID string) GoogleIDTokenVerifier {
	return GoogleIDTokenVerifier{clientID: strings.TrimSpace(clientID)}
}

func (v GoogleIDTokenVerifier) VerifyGoogleIDToken(ctx context.Context, credential string) (GoogleIdentity, error) {
	if v.clientID == "" {
		return GoogleIdentity{}, ErrGoogleAuthDisabled
	}
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return GoogleIdentity{}, ErrInvalidGoogleToken
	}

	payload, err := idtoken.Validate(ctx, credential, v.clientID)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("%w: %v", ErrInvalidGoogleToken, err)
	}

	email, _ := payload.Claims["email"].(string)
	emailVerified, _ := payload.Claims["email_verified"].(bool)
	identity := GoogleIdentity{
		Subject:       strings.TrimSpace(payload.Subject),
		Email:         strings.TrimSpace(email),
		EmailVerified: emailVerified,
	}
	if identity.Subject == "" || identity.Email == "" {
		return GoogleIdentity{}, ErrInvalidGoogleToken
	}
	if !identity.EmailVerified {
		return GoogleIdentity{}, ErrGoogleEmailUnverified
	}

	return identity, nil
}
