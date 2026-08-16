CREATE TABLE IF NOT EXISTS vehicle_restrictions (
    id             TEXT PRIMARY KEY,
    plate          TEXT NOT NULL,
    type           TEXT NOT NULL,                       -- forbidden | manual_confirm
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to   TIMESTAMPTZ NOT NULL,
    reason         TEXT NOT NULL DEFAULT '',
    registered_by  TEXT NOT NULL DEFAULT 'system',
    status         TEXT NOT NULL DEFAULT 'active',      -- active | released
    archived_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    CHECK (effective_to > effective_from),
    CHECK (type IN ('forbidden','manual_confirm')),
    CHECK (status IN ('active','released'))
);
CREATE INDEX IF NOT EXISTS idx_restrictions_plate ON vehicle_restrictions(plate);
CREATE INDEX IF NOT EXISTS idx_restrictions_status ON vehicle_restrictions(status);
CREATE INDEX IF NOT EXISTS idx_restrictions_type ON vehicle_restrictions(type);
CREATE INDEX IF NOT EXISTS idx_restrictions_effective ON vehicle_restrictions(effective_from, effective_to);
-- Supports the active-overlap lookup used by authorization/entry checks and the
-- duplicate-active-window guard. The real overlap enforcement happens inside
-- serializable transactions (same pattern as authorizations); this index keeps
-- those count queries cheap.
CREATE INDEX IF NOT EXISTS idx_restrictions_overlap ON vehicle_restrictions(plate, effective_from, effective_to)
    WHERE archived_at IS NULL AND status = 'active';
