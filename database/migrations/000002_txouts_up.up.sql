CREATE TABLE txouts (
    id          BIGSERIAL   PRIMARY KEY,
    block_hash  BYTEA       NOT NULL REFERENCES block_headers(hash),
    tx_hash     BYTEA       NOT NULL,
    tx_index    INTEGER     NOT NULL,
    value       BIGINT      NOT NULL,
    pk_script   BYTEA       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tx_hash, tx_index)
);
