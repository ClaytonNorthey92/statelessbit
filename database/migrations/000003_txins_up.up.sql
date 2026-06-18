CREATE TABLE txins (
    id                BIGSERIAL   PRIMARY KEY,
    block_hash        BYTEA       NOT NULL REFERENCES block_headers(hash),
    tx_hash           BYTEA       NOT NULL,
    tx_index          INTEGER     NOT NULL,
    prev_out_hash     BYTEA       NOT NULL,
    prev_out_index    BIGINT      NOT NULL,
    script_sig        BYTEA       NOT NULL,
    sequence          BIGINT      NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tx_hash, tx_index)
);
