CREATE TABLE IF NOT EXISTS webhooks (
    id          BIGSERIAL PRIMARY KEY,
    url         TEXT NOT NULL,
    secret      VARCHAR(128),      -- HMAC secret for request signing
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    min_severity VARCHAR(16) NOT NULL DEFAULT 'LOW',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              BIGSERIAL PRIMARY KEY,
    webhook_id      BIGINT NOT NULL REFERENCES webhooks(id),
    alert_id        VARCHAR(64) NOT NULL,
    status_code     INT,
    delivered_at    TIMESTAMPTZ,
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);