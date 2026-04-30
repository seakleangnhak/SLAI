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

	inputCost, err := ceilMulDiv(inputTokens, rule.InputCostUnitsPer1K, 1000)
	if err != nil {
		return 0, PricingRule{}, err
	}
	outputCost, err := ceilMulDiv(outputTokens, rule.OutputCostUnitsPer1K, 1000)
	if err != nil {
		return 0, PricingRule{}, err
	}
	if inputCost > maxInt64()-outputCost {
		return 0, PricingRule{}, errors.New("pricing calculation overflow")
	}
	return inputCost + outputCost, rule, nil
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

func ceilMulDiv(value int64, multiplier int64, divisor int64) (int64, error) {
	if value <= 0 || multiplier <= 0 {
		return 0, nil
	}
	if divisor <= 0 {
		return 0, errors.New("divisor must be positive")
	}
	if value > (maxInt64()-(divisor-1))/multiplier {
		return 0, errors.New("pricing calculation overflow")
	}
	return (value*multiplier + divisor - 1) / divisor, nil
}

func maxInt64() int64 { return int64(^uint64(0) >> 1) }
