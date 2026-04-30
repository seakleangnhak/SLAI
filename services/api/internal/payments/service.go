package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/admin"
	"github.com/slai/slai/services/api/internal/ledger"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var (
	ErrInvalidTopUp        = errors.New("invalid manual top-up")
	ErrIdempotencyConflict = errors.New("idempotency key belongs to a different manual top-up")
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) Service {
	return Service{pool: pool}
}

func (s Service) ListForUser(ctx context.Context, userID string, limit int, offset int) ([]Payment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, package_id::text, provider, provider_ref, amount_minor, currency,
		       credit_units, status, admin_id::text, note, created_at, paid_at
		FROM payments
		WHERE user_id = $1
		ORDER BY paid_at DESC NULLS LAST, created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list user payments: %w", err)
	}
	defer rows.Close()

	items := []Payment{}
	for rows.Next() {
		var payment Payment
		if err := rows.Scan(
			&payment.ID,
			&payment.UserID,
			&payment.PackageID,
			&payment.Provider,
			&payment.ProviderRef,
			&payment.AmountMinor,
			&payment.Currency,
			&payment.CreditUnits,
			&payment.Status,
			&payment.AdminID,
			&payment.Note,
			&payment.CreatedAt,
			&payment.PaidAt,
		); err != nil {
			return nil, fmt.Errorf("scan user payment: %w", err)
		}
		items = append(items, payment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user payments: %w", err)
	}
	return items, nil
}

type ManualTopUpResult struct {
	Payment Payment        `json:"payment"`
	Ledger  ledger.Entry   `json:"ledger"`
	Balance ledger.Balance `json:"balance"`
}

func (s Service) ManualTopUp(ctx context.Context, adminID string, input ManualTopUpInput) (ManualTopUpResult, error) {
	if strings.TrimSpace(input.UserID) == "" || input.AmountMinor < 0 || input.CreditUnits <= 0 {
		return ManualTopUpResult{}, ErrInvalidTopUp
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}

	var result ManualTopUpResult
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if input.IdempotencyKey != nil && *input.IdempotencyKey != "" {
			payment, found, err := findPaymentByProviderRef(ctx, tx, *input.IdempotencyKey)
			if err != nil {
				return err
			}
			if found {
				if payment.UserID != input.UserID || payment.AmountMinor != input.AmountMinor || payment.CreditUnits != input.CreditUnits || payment.Currency != strings.ToUpper(input.Currency) {
					return ErrIdempotencyConflict
				}
				entry, balance, err := findTopUpResult(ctx, tx, payment)
				if err != nil {
					return err
				}
				result = ManualTopUpResult{Payment: payment, Ledger: entry, Balance: balance}
				return nil
			}
		}

		payment, err := insertManualPayment(ctx, tx, adminID, input)
		if err != nil {
			return err
		}

		paymentID := payment.ID
		reason := "manual admin top-up"
		mutation := ledger.Mutation{
			UserID:         input.UserID,
			Type:           ledger.TypePaymentCredit,
			Source:         ledger.SourceAdmin,
			DeltaUnits:     input.CreditUnits,
			PaymentID:      &paymentID,
			AdminID:        &adminID,
			IdempotencyKey: input.IdempotencyKey,
			Reason:         &reason,
			Metadata: map[string]any{
				"amountMinor": input.AmountMinor,
				"currency":    strings.ToUpper(input.Currency),
			},
		}
		entry, balance, err := ledger.NewService(tx).Mutate(ctx, mutation)
		if err != nil {
			return err
		}

		targetType := "payment"
		targetID := payment.ID
		if err := admin.NewAuditLogger(tx).Log(ctx, adminID, "manual_topup_created", &targetType, &targetID, map[string]any{
			"userId":      input.UserID,
			"creditUnits": input.CreditUnits,
			"amountMinor": input.AmountMinor,
			"currency":    strings.ToUpper(input.Currency),
		}); err != nil {
			return err
		}

		result = ManualTopUpResult{Payment: payment, Ledger: entry, Balance: balance}
		return nil
	})
	if err != nil {
		return ManualTopUpResult{}, err
	}

	return result, nil
}

func insertManualPayment(ctx context.Context, tx pgx.Tx, adminID string, input ManualTopUpInput) (Payment, error) {
	var payment Payment
	err := tx.QueryRow(ctx, `
		INSERT INTO payments (
			user_id, package_id, provider, provider_ref, amount_minor, currency,
			credit_units, status, admin_id, note, paid_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		RETURNING id::text, user_id::text, package_id::text, provider, provider_ref, amount_minor, currency,
		          credit_units, status, admin_id::text, note, created_at, paid_at
	`, input.UserID, input.PackageID, ProviderManual, input.IdempotencyKey, input.AmountMinor, strings.ToUpper(input.Currency), input.CreditUnits, StatusPaid, adminID, input.Note).Scan(
		&payment.ID,
		&payment.UserID,
		&payment.PackageID,
		&payment.Provider,
		&payment.ProviderRef,
		&payment.AmountMinor,
		&payment.Currency,
		&payment.CreditUnits,
		&payment.Status,
		&payment.AdminID,
		&payment.Note,
		&payment.CreatedAt,
		&payment.PaidAt,
	)
	if err != nil {
		return Payment{}, fmt.Errorf("insert manual payment: %w", err)
	}
	return payment, nil
}

func findPaymentByProviderRef(ctx context.Context, tx pgx.Tx, providerRef string) (Payment, bool, error) {
	var payment Payment
	err := tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, package_id::text, provider, provider_ref, amount_minor, currency,
		       credit_units, status, admin_id::text, note, created_at, paid_at
		FROM payments
		WHERE provider = $1 AND provider_ref = $2
	`, ProviderManual, providerRef).Scan(
		&payment.ID,
		&payment.UserID,
		&payment.PackageID,
		&payment.Provider,
		&payment.ProviderRef,
		&payment.AmountMinor,
		&payment.Currency,
		&payment.CreditUnits,
		&payment.Status,
		&payment.AdminID,
		&payment.Note,
		&payment.CreatedAt,
		&payment.PaidAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, false, nil
	}
	if err != nil {
		return Payment{}, false, fmt.Errorf("find manual payment by provider ref: %w", err)
	}
	return payment, true, nil
}

func findTopUpResult(ctx context.Context, tx pgx.Tx, payment Payment) (ledger.Entry, ledger.Balance, error) {
	var entry ledger.Entry
	var metadataBytes []byte
	err := tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, type, source, delta_units, balance_after_units,
		       payment_id::text, usage_event_id::text, admin_id::text, idempotency_key, reason, metadata, created_at
		FROM credit_ledger_entries
		WHERE payment_id = $1
	`, payment.ID).Scan(
		&entry.ID,
		&entry.UserID,
		&entry.Type,
		&entry.Source,
		&entry.DeltaUnits,
		&entry.BalanceAfterUnits,
		&entry.PaymentID,
		&entry.UsageEventID,
		&entry.AdminID,
		&entry.IdempotencyKey,
		&entry.Reason,
		&metadataBytes,
		&entry.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Entry{}, ledger.Balance{}, errors.New("idempotent manual top-up missing ledger entry")
	}
	if err != nil {
		return ledger.Entry{}, ledger.Balance{}, err
	}
	if len(metadataBytes) > 0 {
		entry.Metadata = map[string]any{}
		_ = json.Unmarshal(metadataBytes, &entry.Metadata)
	}
	balance, err := ledger.NewService(tx).GetBalance(ctx, payment.UserID)
	if err != nil {
		return ledger.Entry{}, ledger.Balance{}, err
	}
	return entry, balance, nil
}
