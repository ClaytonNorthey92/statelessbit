CREATE INDEX block_headers_hash_idx ON block_headers (hash);
CREATE INDEX block_headers_prev_hash_idx ON block_headers (prev_hash);
CREATE INDEX block_headers_height_idx ON block_headers (height);
