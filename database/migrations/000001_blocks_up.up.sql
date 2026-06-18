CREATE TABLE txouts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    outpoint_index INT NOT NULL CHECK (outpoint_index >= 0),
    outpoint_txhash TEXT NOT NULL,
    UNIQUE (outpoint_index, outpoint_txhash),
    
    created_at_prev_block_hash TEXT NOT NULL,
    created_at_block_hash TEXT NOT NULL,
    created_at_block_height INT NULL,

    owner_address TEXT[] NULL,

    tx_value BIGINT NOT NULL,
    pk_script BYTEA NOT NULL,
    active_block BOOL NOT NULL DEFAULT FALSE
);

CREATE TABLE txout_spends (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    outpoint_index INT NOT NULL CHECK (outpoint_index >= 0),
    outpoint_txhash TEXT NOT NULL,
    UNIQUE (outpoint_index, outpoint_txhash),
    
    spent_at_block_hash TEXT NOT NULL
);

CREATE INDEX txouts_create_idx ON txouts (created_at_block_hash, created_at_block_height, owner_address);

CREATE INDEX txouts_spending_idx ON txout_spends (outpoint_index, outpoint_txhash);
