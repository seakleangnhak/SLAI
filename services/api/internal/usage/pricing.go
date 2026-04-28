package usage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var ErrPricingRuleNotFound = errors.New("no active pricing rule matched usage event")

type PricingService struct {
	db platformdb.Executor
}

func NewPricingService(db platformdb.Executor) PricingService {
	return PricingService{db: db}
}

func (s PricingService) CalculateCost(ctx context.Context, provider *string, model *string, inputTokens int64, outputTokens int64) (int64, PricingRule, error) {
	if inputTokens < 0 || outputTokens < 0 {
		return 0, PricingRule{}, errors.New("token counts cannot be negative")
	}
	rule, err := s.FindRule(ctx, provider, model)
	if err != nil {
		return 0, PricingRule{}, err
	}

	cost := ceilDiv(inputTokens, 1000)*rule.InputCostUnitsPer1K + ceilDiv(outputTokens, 1000)*rule.OutputCostUnitsPer1K
	return cost, rule, nil
}

func (s PricingService) FindRule(ctx context.Context, provider *string, model *string) (PricingRule, error) {
	return scanPricingRule(s.db.QueryRow(ctx, `
		SELECT id::text, provider, model, input_cost_units_per_1k, output_cost_units_per_1k, active, created_at, updated_at
		FROM pricing_rules
		WHERE active = true
		  AND (
		      (provider IS NOT DISTINCT FROM $1::text AND model IS NOT DISTINCT FROM $2::text)
		      OR (provider IS NULL AND model IS NOT DISTINCT FROM $2::text)
		      OR (provider IS NOT DISTINCT FROM $1::text AND model IS NULL)
		      OR (provider IS NULL AND model IS NULL)
		  )
		ORDER BY
		    CASE
		        WHEN provider IS NOT DISTINCT FROM $1::text AND model IS NOT DISTINCT FROM $2::text THEN 1
		        WHEN provider IS NULL AND model IS NOT DISTINCT FROM $2::text THEN 2
		        WHEN provider IS NOT DISTINCT FROM $1::text AND model IS NULL THEN 3
		        WHEN provider IS NULL AND model IS NULL THEN 4
		        ELSE 5
		    END,
		    created_at DESC
		LIMIT 1
	`, provider, model))
}

func scanPricingRule(row pgx.Row) (PricingRule, error) {
	var rule PricingRule
	err := row.Scan(
		&rule.ID,
		&rule.Provider,
		&rule.Model,
		&rule.InputCostUnitsPer1K,
		&rule.OutputCostUnitsPer1K,
		&rule.Active,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PricingRule{}, ErrPricingRuleNotFound
	}
	if err != nil {
		return PricingRule{}, fmt.Errorf("scan pricing rule: %w", err)
	}
	return rule, nil
}

func ceilDiv(value int64, divisor int64) int64 {
	if value <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}
