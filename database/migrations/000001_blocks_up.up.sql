CREATE TABLE txouts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    outpoint_index INT NOT NULL CHECK (outpoint_index >= 0),
    outpoint_txhash TEXT NOT NULL,
    UNIQUE (outpoint_index, outpoint_txhash),
    
    created_at_block_hash TEXT NOT NULL,
    spent_at_block TEXT NULL,

    owner_address TEXT NOT NULL
);

CREATE INDEX txouts_spending_idx ON txouts (created_at_block_hash, spent_at_block, owner_address);
