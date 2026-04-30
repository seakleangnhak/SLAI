package slaipayment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrNotFound       = errors.New("slai payment not found")
	ErrBadStatus      = errors.New("slai payment request failed")
	ErrInvalidConfig  = errors.New("invalid slai payment config")
	ErrInvalidPayload = errors.New("invalid slai payment response")
)

type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

type Client interface {
	CreatePayment(ctx context.Context, input CreatePaymentInput) (Payment, error)
	GetPayment(ctx context.Context, id string) (Payment, error)
	GetPaymentByReference(ctx context.Context, reference string) (Payment, error)
}

type HTTPClient struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
}

func NewHTTPClient(cfg Config) (*HTTPClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("%w: base URL is required", ErrInvalidConfig)
	}
	parsed, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: base URL must be absolute", ErrInvalidConfig)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPClient{
		baseURL: parsed,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

type CreatePaymentInput struct {
	Reference      string         `json:"reference"`
	MerchantPrefix string         `json:"merchant_prefix,omitempty"`
	Amount         string         `json:"amount"`
	Currency       string         `json:"currency"`
	CallbackURL    string         `json:"callback_url"`
	ExpiresIn      string         `json:"expires_in,omitempty"`
	IncludeQRImage *bool          `json:"include_qr_image,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type Payment struct {
	ID             string                 `json:"id"`
	Reference      string                 `json:"reference"`
	MerchantPrefix string                 `json:"merchant_prefix"`
	MerchantName   string                 `json:"merchant_name"`
	Amount         string                 `json:"amount"`
	Currency       string                 `json:"currency"`
	Status         string                 `json:"status"`
	QRPayload      string                 `json:"qr_payload"`
	QRMD5          string                 `json:"qr_md5"`
	QRImageDataURI string                 `json:"qr_image_data_uri,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	ExpiresAt      time.Time              `json:"expires_at"`
	PaidAt         *time.Time             `json:"paid_at,omitempty"`
	Telegram       *TelegramPayment       `json:"telegram,omitempty"`
	Callback       *CallbackDeliveryState `json:"callback,omitempty"`
}

type TelegramPayment struct {
	RawText           string    `json:"raw_text,omitempty"`
	MessageID         int64     `json:"message_id,omitempty"`
	ChatID            int64     `json:"chat_id,omitempty"`
	Amount            string    `json:"amount"`
	Currency          string    `json:"currency"`
	PayerName         string    `json:"payer_name"`
	PayerAccountMask  string    `json:"payer_account_mask"`
	PaidAt            time.Time `json:"paid_at"`
	Method            string    `json:"method"`
	Institution       string    `json:"institution,omitempty"`
	MerchantName      string    `json:"merchant_name"`
	Reference         string    `json:"reference,omitempty"`
	TransactionID     string    `json:"transaction_id"`
	APV               string    `json:"apv"`
	ParseError        string    `json:"parse_error,omitempty"`
	MatchedPaymentID  string    `json:"matched_payment_id,omitempty"`
	CallbackDelivered bool      `json:"callback_delivered"`
}

type CallbackDeliveryState struct {
	DeliveredAt time.Time `json:"delivered_at"`
	StatusCode  int       `json:"status_code,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type CallbackPayload struct {
	Event   string  `json:"event"`
	Payment Payment `json:"payment"`
}

func (c *HTTPClient) CreatePayment(ctx context.Context, input CreatePaymentInput) (Payment, error) {
	var response struct {
		Payment Payment `json:"payment"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/payments", input, &response); err != nil {
		return Payment{}, err
	}
	if response.Payment.ID == "" || response.Payment.Reference == "" {
		return Payment{}, ErrInvalidPayload
	}
	return response.Payment, nil
}

func (c *HTTPClient) GetPayment(ctx context.Context, id string) (Payment, error) {
	var response struct {
		Payment Payment `json:"payment"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/payments/"+url.PathEscape(id), nil, &response); err != nil {
		return Payment{}, err
	}
	if response.Payment.ID == "" {
		return Payment{}, ErrInvalidPayload
	}
	return response.Payment, nil
}

func (c *HTTPClient) GetPaymentByReference(ctx context.Context, reference string) (Payment, error) {
	var response struct {
		Payment Payment `json:"payment"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/payments/by-reference/"+url.PathEscape(reference), nil, &response); err != nil {
		return Payment{}, err
	}
	if response.Payment.ID == "" {
		return Payment{}, ErrInvalidPayload
	}
	return response.Payment, nil
}

func (c *HTTPClient) doJSON(ctx context.Context, method string, path string, input any, output any) error {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: path})
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: status %d: %s", ErrBadStatus, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if output == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(output)
}
