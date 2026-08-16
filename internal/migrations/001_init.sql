CREATE TABLE IF NOT EXISTS residents (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    phone       TEXT NOT NULL,
    building    TEXT NOT NULL,
    unit        TEXT NOT NULL DEFAULT '',
    room        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'active',     -- active | disabled
    archived_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_residents_building ON residents(building);
CREATE INDEX IF NOT EXISTS idx_residents_status ON residents(status);
CREATE TABLE IF NOT EXISTS vehicles (
    id           TEXT PRIMARY KEY,
    plate        TEXT NOT NULL,
    owner_name   TEXT NOT NULL DEFAULT '',
    owner_phone  TEXT NOT NULL DEFAULT '',
    color        TEXT NOT NULL DEFAULT '',
    archived_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicles_plate ON vehicles(plate) WHERE archived_at IS NULL;
CREATE TABLE IF NOT EXISTS parking_areas (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    code        TEXT NOT NULL,
    capacity    INTEGER NOT NULL CHECK (capacity > 0),
    archived_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_areas_code ON parking_areas(code) WHERE archived_at IS NULL;
CREATE TABLE IF NOT EXISTS authorizations (
    id              TEXT PRIMARY KEY,
    resident_id     TEXT NOT NULL REFERENCES residents(id),
    plate           TEXT NOT NULL,
    parking_area_id TEXT NOT NULL REFERENCES parking_areas(id),
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending', -- pending|active|completed|cancelled
    purpose         TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL DEFAULT 'system',
    archived_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    CHECK (end_time > start_time)
);
CREATE INDEX IF NOT EXISTS idx_auth_plate ON authorizations(plate);
CREATE INDEX IF NOT EXISTS idx_auth_status ON authorizations(status);
CREATE INDEX IF NOT EXISTS idx_auth_area ON authorizations(parking_area_id);
CREATE INDEX IF NOT EXISTS idx_auth_resident ON authorizations(resident_id);
CREATE INDEX IF NOT EXISTS idx_auth_start ON authorizations(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_auth_overlap ON authorizations(plate, start_time, end_time)
    WHERE archived_at IS NULL AND status IN ('pending','active');
CREATE TABLE IF NOT EXISTS entry_exit_records (
    id               TEXT PRIMARY KEY,
    authorization_id TEXT NOT NULL REFERENCES authorizations(id),
    plate            TEXT NOT NULL,
    parking_area_id  TEXT NOT NULL,
    entry_time       TIMESTAMPTZ NOT NULL,
    exit_time        TIMESTAMPTZ,
    exit_operator    TEXT NOT NULL DEFAULT '',
    exit_note        TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'entered', -- entered | exited
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_records_active_auth ON entry_exit_records(authorization_id)
    WHERE status = 'entered';
CREATE INDEX IF NOT EXISTS idx_records_area_status ON entry_exit_records(parking_area_id, status);
CREATE INDEX IF NOT EXISTS idx_records_entry ON entry_exit_records(entry_time DESC);
CREATE TABLE IF NOT EXISTS audit_logs (
    id          TEXT PRIMARY KEY,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    operator    TEXT NOT NULL DEFAULT 'system',
    detail      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_logs(entity_type, created_at DESC);
