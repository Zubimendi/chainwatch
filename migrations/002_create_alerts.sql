CREATE TABLE IF NOT EXISTS alerts (
    id               VARCHAR(64) PRIMARY KEY,
    type             VARCHAR(64) NOT NULL,
    severity         VARCHAR(16) NOT NULL,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL,
    transaction_hash VARCHAR(66),
    from_address     VARCHAR(42),
    to_address       VARCHAR(42),
    value_eth        NUMERIC(30, 18),
    gas_price_gwei   NUMERIC(20, 9),
    block_number     BIGINT,
    triggered_rule   VARCHAR(128) NOT NULL,
    rule_metadata    JSONB,
    detected_at      TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alerts_type        ON alerts(type);
CREATE INDEX idx_alerts_severity    ON alerts(severity);
CREATE INDEX idx_alerts_detected_at ON alerts(detected_at DESC);
CREATE INDEX idx_alerts_from_addr   ON alerts(from_address);
CREATE INDEX idx_alerts_tx_hash     ON alerts(transaction_hash);