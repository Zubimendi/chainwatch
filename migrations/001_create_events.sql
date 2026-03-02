CREATE TABLE IF NOT EXISTS decoded_transactions (
    id              BIGSERIAL PRIMARY KEY,
    hash            VARCHAR(66) NOT NULL UNIQUE,
    from_address    VARCHAR(42) NOT NULL,
    to_address      VARCHAR(42),
    value_wei       NUMERIC(78, 0) NOT NULL DEFAULT 0,
    value_eth       NUMERIC(30, 18) NOT NULL DEFAULT 0,
    gas_limit       BIGINT,
    gas_price_gwei  NUMERIC(20, 9),
    nonce           BIGINT,
    block_number    BIGINT,
    method_name     VARCHAR(128),
    is_deployment   BOOLEAN NOT NULL DEFAULT FALSE,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tx_from_address  ON decoded_transactions(from_address);
CREATE INDEX idx_tx_to_address    ON decoded_transactions(to_address);
CREATE INDEX idx_tx_block_number  ON decoded_transactions(block_number);
CREATE INDEX idx_tx_received_at   ON decoded_transactions(received_at DESC);