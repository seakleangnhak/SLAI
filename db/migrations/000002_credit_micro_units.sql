CREATE TABLE IF NOT EXISTS schema_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM schema_settings
        WHERE key = 'credit_unit_scale' AND value = '1000000'
    ) THEN
        PERFORM set_config('slai.ledger_mutation', 'on', true);

        UPDATE credit_packages
        SET credit_units = credit_units * 1000000,
            bonus_credit_units = bonus_credit_units * 1000000;

        UPDATE payments
        SET credit_units = credit_units * 1000000;

        UPDATE usage_events
        SET cost_units = cost_units * 1000000;

        UPDATE credit_ledger_entries
        SET delta_units = delta_units * 1000000,
            balance_after_units = balance_after_units * 1000000;

        UPDATE credit_balances
        SET available_units = available_units * 1000000,
            lifetime_purchased_units = lifetime_purchased_units * 1000000,
            lifetime_used_units = lifetime_used_units * 1000000;

        UPDATE pricing_rules
        SET input_cost_units_per_1k = input_cost_units_per_1k * 1000000,
            output_cost_units_per_1k = output_cost_units_per_1k * 1000000;

        INSERT INTO schema_settings (key, value)
        VALUES ('credit_unit_scale', '1000000')
        ON CONFLICT (key) DO UPDATE
        SET value = EXCLUDED.value,
            updated_at = now();
    END IF;
END $$;
