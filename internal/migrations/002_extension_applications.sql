CREATE TABLE IF NOT EXISTS extension_applications (
    id                TEXT PRIMARY KEY,
    authorization_id  TEXT NOT NULL REFERENCES authorizations(id),
    plate             TEXT NOT NULL,
    original_end_time TIMESTAMPTZ NOT NULL,
    new_end_time      TIMESTAMPTZ NOT NULL,
    reason            TEXT NOT NULL DEFAULT '',   -- 延期原因（申请人填写）
    applicant         TEXT NOT NULL DEFAULT '',   -- 申请人
    status            TEXT NOT NULL DEFAULT 'pending', -- pending|approved|rejected|revoked
    decided_by        TEXT NOT NULL DEFAULT '',   -- 审批人/驳回人/撤销人
    decided_at        TIMESTAMPTZ,                -- 审批/决策时间
    decision_note     TEXT NOT NULL DEFAULT '',   -- 审批备注/驳回原因/撤销原因
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,
    CHECK (new_end_time > original_end_time)
);
CREATE INDEX IF NOT EXISTS idx_extapp_auth ON extension_applications(authorization_id);
CREATE INDEX IF NOT EXISTS idx_extapp_status ON extension_applications(status);
CREATE INDEX IF NOT EXISTS idx_extapp_plate ON extension_applications(plate);
CREATE INDEX IF NOT EXISTS idx_extapp_applicant ON extension_applications(applicant);
CREATE INDEX IF NOT EXISTS idx_extapp_created ON extension_applications(created_at DESC);
-- A single authorization may have at most one pending application at a time.
CREATE UNIQUE INDEX IF NOT EXISTS idx_extapp_pending_auth
    ON extension_applications(authorization_id) WHERE status = 'pending';
