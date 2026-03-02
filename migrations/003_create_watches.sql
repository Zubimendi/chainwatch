CREATE TABLE IF NOT EXISTS watched_addresses (
    id          BIGSERIAL PRIMARY KEY,
    address     VARCHAR(42) NOT NULL UNIQUE,
    label       VARCHAR(128),
    notes       TEXT,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);