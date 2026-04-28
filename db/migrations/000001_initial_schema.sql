CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('USER', 'ADMIN')),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    balance_policy TEXT NOT NULL DEFAULT 'allow_overdraft_until_sync',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER users_set_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_token_hash_idx ON sessions(token_hash);

CREATE TABLE credit_packages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NULL,
    credit_units BIGINT NOT NULL CHECK (credit_units > 0),
    bonus_credit_units BIGINT NOT NULL DEFAULT 0 CHECK (bonus_credit_units >= 0),
    price_minor BIGINT NOT NULL CHECK (price_minor >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    active BOOLEAN NOT NULL DEFAULT true,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER credit_packages_set_updated_at
BEFORE UPDATE ON credit_packages
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    package_id UUID NULL REFERENCES credit_packages(id) ON DELETE SET NULL,
    provider TEXT NOT NULL DEFAULT 'manual',
    provider_ref TEXT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency TEXT NOT NULL,
    credit_units BIGINT NOT NULL CHECK (credit_units > 0),
    status TEXT NOT NULL,
    admin_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    note TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ NULL
);

CREATE INDEX payments_user_id_idx ON payments(user_id);
CREATE INDEX payments_admin_id_idx ON payments(admin_id);
CREATE UNIQUE INDEX payments_provider_ref_unique_idx
ON payments(provider, provider_ref)
WHERE provider_ref IS NOT NULL;

CREATE TABLE credit_balances (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    available_units BIGINT NOT NULL DEFAULT 0,
    lifetime_purchased_units BIGINT NOT NULL DEFAULT 0 CHECK (lifetime_purchased_units >= 0),
    lifetime_used_units BIGINT NOT NULL DEFAULT 0 CHECK (lifetime_used_units >= 0),
    version BIGINT NOT NULL DEFAULT 0 CHECK (version >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION prevent_direct_credit_balance_mutation()
RETURNS TRIGGER AS $$
BEGIN
    IF current_setting('slai.ledger_mutation', true) <> 'on' THEN
        RAISE EXCEPTION 'credit_balances may only be mutated through the ledger service';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER credit_balances_guard_mutation
BEFORE INSERT OR UPDATE OR DELETE ON credit_balances
FOR EACH ROW EXECUTE FUNCTION prevent_direct_credit_balance_mutation();

CREATE TRIGGER credit_balances_set_updated_at
BEFORE UPDATE ON credit_balances
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE credit_ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    type TEXT NOT NULL CHECK (type IN (
        'payment_credit',
        'usage_debit',
        'admin_adjustment_credit',
        'admin_adjustment_debit',
        'refund_debit',
        'bonus_credit'
    )),
    source TEXT NOT NULL,
    delta_units BIGINT NOT NULL CHECK (delta_units <> 0),
    balance_after_units BIGINT NOT NULL,
    payment_id UUID NULL REFERENCES payments(id) ON DELETE SET NULL,
    usage_event_id UUID NULL,
    admin_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    idempotency_key TEXT UNIQUE NULL,
    reason TEXT NULL,
    metadata JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX credit_ledger_entries_user_id_idx ON credit_ledger_entries(user_id);
CREATE INDEX credit_ledger_entries_payment_id_idx ON credit_ledger_entries(payment_id);
CREATE INDEX credit_ledger_entries_usage_event_id_idx ON credit_ledger_entries(usage_event_id);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    omniroute_key_id TEXT NULL,
    key_hash TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'SUSPENDED', 'REVOKED')),
    last_used_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX api_keys_one_active_per_user_idx
ON api_keys(user_id)
WHERE status = 'ACTIVE';
CREATE INDEX api_keys_user_id_idx ON api_keys(user_id);
CREATE INDEX api_keys_key_hash_idx ON api_keys(key_hash);
CREATE INDEX api_keys_omniroute_key_id_idx ON api_keys(omniroute_key_id);

CREATE TRIGGER api_keys_set_updated_at
BEFORE UPDATE ON api_keys
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE usage_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
    external_source TEXT NOT NULL,
    external_event_id TEXT NOT NULL,
    omniroute_key_id TEXT NULL,
    model TEXT NULL,
    provider TEXT NULL,
    input_tokens BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    total_tokens BIGINT NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    cost_units BIGINT NOT NULL CHECK (cost_units >= 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'billed', 'duplicate', 'failed', 'ignored')),
    occurred_at TIMESTAMPTZ NOT NULL,
    raw_json JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (external_source, external_event_id)
);

CREATE INDEX usage_events_user_id_idx ON usage_events(user_id);
CREATE INDEX usage_events_api_key_id_idx ON usage_events(api_key_id);
CREATE INDEX usage_events_status_idx ON usage_events(status);
CREATE INDEX usage_events_occurred_at_idx ON usage_events(occurred_at);

CREATE TABLE pricing_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider TEXT NULL,
    model TEXT NULL,
    input_cost_units_per_1k BIGINT NOT NULL CHECK (input_cost_units_per_1k >= 0),
    output_cost_units_per_1k BIGINT NOT NULL CHECK (output_cost_units_per_1k >= 0),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX pricing_rules_lookup_idx
ON pricing_rules(provider, model)
WHERE active = true;

CREATE TRIGGER pricing_rules_set_updated_at
BEFORE UPDATE ON pricing_rules
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE admin_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action TEXT NOT NULL,
    target_type TEXT NULL,
    target_id TEXT NULL,
    metadata JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX admin_audit_logs_admin_id_idx ON admin_audit_logs(admin_id);
CREATE INDEX admin_audit_logs_target_idx ON admin_audit_logs(target_type, target_id);

CREATE TABLE omniroute_sync_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT UNIQUE NOT NULL,
    last_seen_timestamp TIMESTAMPTZ NULL,
    last_seen_external_id TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER omniroute_sync_state_set_updated_at
BEFORE UPDATE ON omniroute_sync_state
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE credit_ledger_entries
ADD CONSTRAINT credit_ledger_entries_usage_event_id_fkey
FOREIGN KEY (usage_event_id)
REFERENCES usage_events(id)
ON DELETE SET NULL;
