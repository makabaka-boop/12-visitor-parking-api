CREATE TABLE IF NOT EXISTS billing_rules (
    id                  TEXT PRIMARY KEY,
    parking_area_id     TEXT NOT NULL REFERENCES parking_areas(id),
    free_minutes        INTEGER NOT NULL DEFAULT 0 CHECK (free_minutes >= 0),
    hourly_rate_cents   BIGINT  NOT NULL CHECK (hourly_rate_cents > 0),
    daily_cap_cents     BIGINT  NOT NULL DEFAULT 0 CHECK (daily_cap_cents >= 0),
    effective_from      TIMESTAMPTZ NOT NULL,
    status              TEXT NOT NULL DEFAULT 'active',   -- active | archived
    archived_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);
-- one active rule per (area, effective_from); rules are versioned over time
CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_rules_area_eff
    ON billing_rules(parking_area_id, effective_from) WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_billing_rules_area
    ON billing_rules(parking_area_id, effective_from DESC) WHERE archived_at IS NULL;

CREATE TABLE IF NOT EXISTS fees (
    id                TEXT PRIMARY KEY,
    record_id         TEXT NOT NULL REFERENCES entry_exit_records(id),
    authorization_id  TEXT NOT NULL,
    plate             TEXT NOT NULL,
    parking_area_id   TEXT NOT NULL,
    billing_rule_id   TEXT NOT NULL DEFAULT '',
    entry_time        TIMESTAMPTZ NOT NULL,
    exit_time         TIMESTAMPTZ NOT NULL,
    duration_minutes  BIGINT NOT NULL DEFAULT 0,
    charged_minutes   BIGINT NOT NULL DEFAULT 0,
    amount_cents      BIGINT NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'unsettled',  -- unsettled | settled
    settle_method     TEXT NOT NULL DEFAULT '',           -- cash | online | waiver
    settle_reason     TEXT NOT NULL DEFAULT '',
    settle_operator   TEXT NOT NULL DEFAULT '',
    settled_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,
    CHECK (status IN ('unsettled','settled')),
    CHECK (settle_method IN ('','cash','online','waiver'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fees_record ON fees(record_id);
CREATE INDEX IF NOT EXISTS idx_fees_status ON fees(status);
CREATE INDEX IF NOT EXISTS idx_fees_area ON fees(parking_area_id);
CREATE INDEX IF NOT EXISTS idx_fees_plate ON fees(plate);
