package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/slai/slai/services/api/internal/credits"
)

const (
	defaultBrevoAPIURL      = "https://api.brevo.com/v3/smtp/email"
	defaultEmailSendTimeout = 10 * time.Second
)

type EmailSender interface {
	SendSignupOTP(ctx context.Context, email, otp string, expiresAt time.Time) error
	SendPasswordResetOTP(ctx context.Context, email, otp string, expiresAt time.Time) error
	SendPasswordChangedAlert(ctx context.Context, email string, changedAt time.Time) error
	SendLowBalanceAlert(ctx context.Context, email string, balanceUnits, thresholdUnits int64) error
}

type NoopEmailSender struct {
	Log *slog.Logger
}

func (s NoopEmailSender) SendSignupOTP(_ context.Context, email, otp string, expiresAt time.Time) error {
	if s.Log != nil {
		s.Log.Warn("email delivery is not configured; signup email verification OTP generated", "email", email, "otp", otp, "expires_at", expiresAt)
	}
	return nil
}

func (s NoopEmailSender) SendPasswordResetOTP(_ context.Context, email, otp string, expiresAt time.Time) error {
	if s.Log != nil {
		s.Log.Warn("email delivery is not configured; password reset OTP generated", "email", email, "otp", otp, "expires_at", expiresAt)
	}
	return nil
}

func (s NoopEmailSender) SendPasswordChangedAlert(_ context.Context, email string, changedAt time.Time) error {
	if s.Log != nil {
		s.Log.Warn("email delivery is not configured; password changed alert generated", "email", email, "changed_at", changedAt)
	}
	return nil
}

func (s NoopEmailSender) SendLowBalanceAlert(_ context.Context, email string, balanceUnits, thresholdUnits int64) error {
	if s.Log != nil {
		s.Log.Warn("email delivery is not configured; low balance alert generated", "email", email, "balance_units", balanceUnits, "threshold_units", thresholdUnits)
	}
	return nil
}

type BrevoConfig struct {
	APIKey  string
	APIURL  string
	From    string
	Timeout time.Duration
}

type BrevoEmailSender struct {
	cfg    BrevoConfig
	client *http.Client
}

func NewBrevoEmailSender(cfg BrevoConfig) BrevoEmailSender {
	if strings.TrimSpace(cfg.APIURL) == "" {
		cfg.APIURL = defaultBrevoAPIURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultEmailSendTimeout
	}
	return BrevoEmailSender{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (s BrevoEmailSender) SendSignupOTP(ctx context.Context, email, otp string, expiresAt time.Time) error {
	return s.sendOTP(ctx, email, "Verify your SLAI email", signupOTPText(otp, expiresAt), "signup OTP")
}

func (s BrevoEmailSender) SendPasswordResetOTP(ctx context.Context, email, otp string, expiresAt time.Time) error {
	return s.sendOTP(ctx, email, "Reset your SLAI password", passwordResetOTPText(otp, expiresAt), "password reset OTP")
}

func (s BrevoEmailSender) SendPasswordChangedAlert(ctx context.Context, email string, changedAt time.Time) error {
	return s.sendOTP(ctx, email, "Your SLAI password was changed", passwordChangedAlertText(changedAt), "password changed alert")
}

func (s BrevoEmailSender) SendLowBalanceAlert(ctx context.Context, email string, balanceUnits, thresholdUnits int64) error {
	return s.sendOTP(ctx, email, "Your SLAI balance is low", lowBalanceAlertText(balanceUnits, thresholdUnits), "low balance alert")
}

func (s BrevoEmailSender) sendOTP(ctx context.Context, email, subject, textContent, label string) error {
	apiKey := strings.TrimSpace(s.cfg.APIKey)
	from := strings.TrimSpace(s.cfg.From)
	if apiKey == "" || from == "" {
		return fmt.Errorf("Brevo API key and sender address are required")
	}

	payload := brevoSendEmailRequest{
		Sender: brevoEmailAddress{
			Name:  "SLAI",
			Email: from,
		},
		To:          []brevoEmailAddress{{Email: email}},
		Subject:     subject,
		TextContent: textContent,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Brevo email payload: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, s.cfg.APIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Brevo email request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send %s email through Brevo: %w", label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("send %s email through Brevo: status %d: %s", label, resp.StatusCode, message)
	}

	return nil
}

type brevoSendEmailRequest struct {
	Sender      brevoEmailAddress   `json:"sender"`
	To          []brevoEmailAddress `json:"to"`
	Subject     string              `json:"subject"`
	TextContent string              `json:"textContent"`
}

type brevoEmailAddress struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	Timeout  time.Duration
}

type SMTPEmailSender struct {
	cfg SMTPConfig
}

func NewSMTPEmailSender(cfg SMTPConfig) SMTPEmailSender {
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultEmailSendTimeout
	}
	return SMTPEmailSender{cfg: cfg}
}

func (s SMTPEmailSender) SendSignupOTP(ctx context.Context, email, otp string, expiresAt time.Time) error {
	return s.sendOTP(ctx, email, "Verify your SLAI email", signupOTPText(otp, expiresAt), "signup OTP")
}

func (s SMTPEmailSender) SendPasswordResetOTP(ctx context.Context, email, otp string, expiresAt time.Time) error {
	return s.sendOTP(ctx, email, "Reset your SLAI password", passwordResetOTPText(otp, expiresAt), "password reset OTP")
}

func (s SMTPEmailSender) SendPasswordChangedAlert(ctx context.Context, email string, changedAt time.Time) error {
	return s.sendOTP(ctx, email, "Your SLAI password was changed", passwordChangedAlertText(changedAt), "password changed alert")
}

func (s SMTPEmailSender) SendLowBalanceAlert(ctx context.Context, email string, balanceUnits, thresholdUnits int64) error {
	return s.sendOTP(ctx, email, "Your SLAI balance is low", lowBalanceAlertText(balanceUnits, thresholdUnits), "low balance alert")
}

func (s SMTPEmailSender) sendOTP(ctx context.Context, email, subject, body, label string) error {
	host := strings.TrimSpace(s.cfg.Host)
	from := strings.TrimSpace(s.cfg.From)
	if host == "" || from == "" {
		return fmt.Errorf("SMTP host and from address are required")
	}

	addr := fmt.Sprintf("%s:%d", host, s.cfg.Port)
	message := strings.Join([]string{
		"From: " + from,
		"To: " + email,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	var auth smtp.Auth
	if strings.TrimSpace(s.cfg.Username) != "" || strings.TrimSpace(s.cfg.Password) != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, host)
	}

	if err := sendSMTPMail(ctx, addr, host, auth, from, []string{email}, []byte(message), s.cfg.Timeout); err != nil {
		return fmt.Errorf("send %s email: %w", label, err)
	}
	return nil
}

func signupOTPText(otp string, expiresAt time.Time) string {
	return fmt.Sprintf("Your SLAI verification code is %s. It expires at %s.\n\nIf you did not request this, you can ignore this email.\n", otp, expiresAt.UTC().Format(time.RFC1123))
}

func passwordResetOTPText(otp string, expiresAt time.Time) string {
	return fmt.Sprintf("Your SLAI password reset code is %s. It expires at %s.\n\nIf you did not request this, you can ignore this email.\n", otp, expiresAt.UTC().Format(time.RFC1123))
}

func passwordChangedAlertText(changedAt time.Time) string {
	return fmt.Sprintf("Your SLAI password was changed at %s.\n\nIf this was not you, reset your password immediately and contact support.\n", changedAt.UTC().Format(time.RFC1123))
}

func lowBalanceAlertText(balanceUnits, thresholdUnits int64) string {
	return fmt.Sprintf("Your SLAI balance is low. Current balance: %s credits. Alert threshold: %s credits.\n\nAdd credits to keep API access available.\n", formatCreditUnits(balanceUnits), formatCreditUnits(thresholdUnits))
}

func formatCreditUnits(units int64) string {
	negative := units < 0
	if negative {
		units = -units
	}
	whole := units / credits.Scale
	fraction := units % credits.Scale
	formatted := fmt.Sprintf("%d", whole)
	if fraction > 0 {
		fractionText := strings.TrimRight(fmt.Sprintf("%06d", fraction), "0")
		formatted += "." + fractionText
	}
	if negative {
		return "-" + formatted
	}
	return formatted
}

func sendSMTPMail(ctx context.Context, addr, host string, auth smtp.Auth, from string, to []string, msg []byte, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultEmailSendTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(requestCtx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := requestCtx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		_ = wc.Close()
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return client.Quit()
}
