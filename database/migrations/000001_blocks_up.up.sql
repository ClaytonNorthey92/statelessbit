-- Querying by hash using a hex string (e.g. from mempool.space):
--
--   SELECT * FROM block_headers WHERE hash = decode('<hex>', 'hex');
--   SELECT * FROM block_headers WHERE hash = '\x<hex>';
--
-- Note: mempool.space displays hashes in reversed byte order relative to
-- Bitcoin's internal representation. If hashes are stored in internal
-- (little-endian) byte order, reverse the bytes before comparing:
--
--   CREATE OR REPLACE FUNCTION reverse_bytes(b BYTEA) RETURNS BYTEA AS $$
--     SELECT string_agg(byte, '' ORDER BY n DESC)::bytea
--     FROM (SELECT n, substring(b FROM n FOR 1) AS byte
--           FROM generate_series(1, length(b)) AS n) sub;
--   $$ LANGUAGE sql IMMUTABLE STRICT;
--
--   SELECT * FROM block_headers WHERE hash = reverse_bytes(decode('<hex>', 'hex'));

CREATE TABLE block_headers (
    id              BIGSERIAL PRIMARY KEY,
    hash            BYTEA        NOT NULL UNIQUE,
    version         INTEGER      NOT NULL,
    prev_hash       BYTEA        NOT NULL,
    merkle_root     BYTEA        NOT NULL,
    timestamp       TIMESTAMPTZ  NOT NULL,
    bits            BIGINT       NOT NULL,
    nonce           BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
