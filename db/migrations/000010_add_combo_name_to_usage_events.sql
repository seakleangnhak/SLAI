ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS combo_name TEXT NULL;

UPDATE usage_events
SET combo_name = COALESCE(
    NULLIF(BTRIM(raw_json->>'comboName'), ''),
    NULLIF(BTRIM(raw_json->>'combo_name'), '')
)
WHERE combo_name IS NULL
  AND raw_json IS NOT NULL;
