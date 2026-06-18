package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"
	"mydatabase"
)

const (
	pgUniqueViolation  pq.ErrorCode = "23505"
	pgNotNullViolation pq.ErrorCode = "23502"
)

func TestInsertBlockHeader(t *testing.T) {
	validBlock := &BlockHeader{
		Hash:       []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Version:    1,
		PrevHash:   []byte("00000000000000000000000000000000"),
		MerkleRoot: []byte("mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"),
		Timestamp:  time.Now().UTC().Truncate(time.Microsecond),
		Bits:       0x1d00ffff,
		Nonce:      2083236893,
	}

	tests := []struct {
		name        string
		block       *BlockHeader
		setup       func(ctx context.Context, db *sql.DB)
		wantErrCode pq.ErrorCode
	}{
		{
			name:  "insert new block succeeds",
			block: validBlock,
		},
		{
			name: "nil hash is rejected",
			block: &BlockHeader{
				Hash:       nil,
				Version:    1,
				PrevHash:   []byte("00000000000000000000000000000000"),
				MerkleRoot: []byte("mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"),
				Timestamp:  time.Now().UTC().Truncate(time.Microsecond),
				Bits:       0x1d00ffff,
				Nonce:      2083236893,
			},
			wantErrCode: pgNotNullViolation,
		},
		{
			name: "nil prev_hash is rejected",
			block: &BlockHeader{
				Hash:       []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				Version:    1,
				PrevHash:   nil,
				MerkleRoot: []byte("mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"),
				Timestamp:  time.Now().UTC().Truncate(time.Microsecond),
				Bits:       0x1d00ffff,
				Nonce:      2083236893,
			},
			wantErrCode: pgNotNullViolation,
		},
		{
			name: "nil merkle_root is rejected",
			block: &BlockHeader{
				Hash:       []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				Version:    1,
				PrevHash:   []byte("00000000000000000000000000000000"),
				MerkleRoot: nil,
				Timestamp:  time.Now().UTC().Truncate(time.Microsecond),
				Bits:       0x1d00ffff,
				Nonce:      2083236893,
			},
			wantErrCode: pgNotNullViolation,
		},
		{
			name: "duplicate block hash is rejected",
			setup: func(ctx context.Context, db *sql.DB) {
				if err := InsertBlockHeader(ctx, db, validBlock); err != nil {
					t.Fatalf("setup: pre-insert failed: %s", err)
				}
			},
			block:       validBlock,
			wantErrCode: pgUniqueViolation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, drop, err := database.CreateNewRandomDatabase(t.Context())
			if err != nil {
				t.Fatalf("could not create test database: %s", err)
			}
			defer drop()

			if tt.setup != nil {
				tt.setup(t.Context(), db)
			}

			err = InsertBlockHeader(t.Context(), db, tt.block)

			if tt.wantErrCode == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}

			var pqErr *pq.Error
			if !errors.As(err, &pqErr) {
				t.Fatalf("expected *pq.Error, got: %v", err)
			}
			if pqErr.Code != tt.wantErrCode {
				t.Errorf("error code = %q, want %q", pqErr.Code, tt.wantErrCode)
			}
		})
	}
}
