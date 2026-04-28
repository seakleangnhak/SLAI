package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/ledger"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var ErrInvalidAdjustment = errors.New("invalid credit adjustment")

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) Service {
	return Service{pool: pool}
}

type AdjustmentInput struct {
	UserID         string  `json:"userId"`
	DeltaUnits     int64   `json:"deltaUnits"`
	Reason         string  `json:"reason"`
	IdempotencyKey *string `json:"idempotencyKey"`
}

type AdjustmentResult struct {
	Ledger  ledger.Entry   `json:"ledger"`
	Balance ledger.Balance `json:"balance"`
}

func (s Service) AdjustCredits(ctx context.Context, adminID string, input AdjustmentInput) (AdjustmentResult, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.UserID == "" || input.DeltaUnits == 0 || input.Reason == "" {
		return AdjustmentResult{}, ErrInvalidAdjustment
	}

	mutationType := ledger.TypeAdminAdjustmentCredit
	if input.DeltaUnits < 0 {
		mutationType = ledger.TypeAdminAdjustmentDebit
	}

	var result AdjustmentResult
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		preexisting := false
		ledgerService := ledger.NewService(tx)
		if input.IdempotencyKey != nil && *input.IdempotencyKey != "" {
			_, found, err := ledgerService.GetByIdempotencyKey(ctx, *input.IdempotencyKey)
			if err != nil {
				return err
			}
			preexisting = found
		}

		entry, balance, err := ledgerService.Mutate(ctx, ledger.Mutation{
			UserID:         input.UserID,
			Type:           mutationType,
			Source:         ledger.SourceAdmin,
			DeltaUnits:     input.DeltaUnits,
			AdminID:        &adminID,
			IdempotencyKey: input.IdempotencyKey,
			Reason:         &input.Reason,
		})
		if err != nil {
			return err
		}

		if !preexisting {
			targetType := "user"
			targetID := input.UserID
			if err := NewAuditLogger(tx).Log(ctx, adminID, "credit_adjustment_created", &targetType, &targetID, map[string]any{
				"deltaUnits": input.DeltaUnits,
				"reason":     input.Reason,
			}); err != nil {
				return err
			}
		}

		result = AdjustmentResult{Ledger: entry, Balance: balance}
		return nil
	})
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("adjust credits: %w", err)
	}

	return result, nil
}
