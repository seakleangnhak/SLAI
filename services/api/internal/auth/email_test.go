package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrevoEmailSenderSendsSignupOTP(t *testing.T) {
	t.Helper()

	var payload brevoSendEmailRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("api-key"); got != "test-api-key" {
			t.Fatalf("api-key header = %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("content-type = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		writeJSON(t, w, http.StatusCreated, map[string]string{"messageId": "queued"})
	}))
	defer server.Close()

	sender := NewBrevoEmailSender(BrevoConfig{
		APIKey:  "test-api-key",
		APIURL:  server.URL,
		From:    "noreply@slai.shop",
		Timeout: time.Second,
	})

	expiresAt := time.Date(2026, 5, 19, 4, 15, 0, 0, time.UTC)
	if err := sender.SendSignupOTP(context.Background(), "dev@example.com", "123456", expiresAt); err != nil {
		t.Fatal(err)
	}
	if payload.Sender.Email != "noreply@slai.shop" {
		t.Fatalf("sender email = %q", payload.Sender.Email)
	}
	if len(payload.To) != 1 || payload.To[0].Email != "dev@example.com" {
		t.Fatalf("to = %#v", payload.To)
	}
	if payload.Subject != "Verify your SLAI email" {
		t.Fatalf("subject = %q", payload.Subject)
	}
	if !strings.Contains(payload.TextContent, "123456") {
		t.Fatalf("text content did not include OTP: %q", payload.TextContent)
	}
}

func TestBrevoEmailSenderStatusErrorDoesNotLeakAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
	}))
	defer server.Close()

	sender := NewBrevoEmailSender(BrevoConfig{
		APIKey:  "secret-api-key",
		APIURL:  server.URL,
		From:    "noreply@slai.shop",
		Timeout: time.Second,
	})

	err := sender.SendSignupOTP(context.Background(), "dev@example.com", "123456", time.Now().UTC())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("error = %q", err.Error())
	}
	if strings.Contains(err.Error(), "secret-api-key") {
		t.Fatalf("error leaked API key: %q", err.Error())
	}
}

func TestBrevoEmailSenderSendsPasswordResetOTP(t *testing.T) {
	var payload brevoSendEmailRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		writeJSON(t, w, http.StatusCreated, map[string]string{"messageId": "queued"})
	}))
	defer server.Close()

	sender := NewBrevoEmailSender(BrevoConfig{
		APIKey:  "test-api-key",
		APIURL:  server.URL,
		From:    "noreply@slai.shop",
		Timeout: time.Second,
	})

	expiresAt := time.Date(2026, 5, 19, 4, 15, 0, 0, time.UTC)
	if err := sender.SendPasswordResetOTP(context.Background(), "dev@example.com", "654321", expiresAt); err != nil {
		t.Fatal(err)
	}
	if payload.Subject != "Reset your SLAI password" {
		t.Fatalf("subject = %q", payload.Subject)
	}
	if !strings.Contains(payload.TextContent, "654321") || !strings.Contains(payload.TextContent, "password reset code") {
		t.Fatalf("text content did not include reset OTP: %q", payload.TextContent)
	}
}

func TestBrevoEmailSenderSendsSecurityNotifications(t *testing.T) {
	payloads := []brevoSendEmailRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload brevoSendEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		payloads = append(payloads, payload)
		writeJSON(t, w, http.StatusCreated, map[string]string{"messageId": "queued"})
	}))
	defer server.Close()

	sender := NewBrevoEmailSender(BrevoConfig{
		APIKey:  "test-api-key",
		APIURL:  server.URL,
		From:    "noreply@slai.shop",
		Timeout: time.Second,
	})

	if err := sender.SendPasswordChangedAlert(context.Background(), "dev@example.com", time.Date(2026, 5, 19, 4, 15, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := sender.SendLowBalanceAlert(context.Background(), "dev@example.com", 1_250_000, 5_000_000); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 2 {
		t.Fatalf("payload count = %d", len(payloads))
	}
	if payloads[0].Subject != "Your SLAI password was changed" || !strings.Contains(payloads[0].TextContent, "password was changed") {
		t.Fatalf("password alert payload = %#v", payloads[0])
	}
	if payloads[1].Subject != "Your SLAI balance is low" || !strings.Contains(payloads[1].TextContent, "1.25 credits") {
		t.Fatalf("low balance payload = %#v", payloads[1])
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
