package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/lib/pq"
	"mydatabase"
)

const (
	pgUniqueViolation  pq.ErrorCode = "23505"
	pgNotNullViolation pq.ErrorCode = "23502"
	pgFKViolation      pq.ErrorCode = "23503"
)

func TestInsertTxOut(t *testing.T) {
	validBlockHash := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	validTxHash := []byte("tttttttttttttttttttttttttttttttt")
	// P2WPKH script (OP_0 <20-byte-hash>) so address extraction succeeds.
	validTxOut := &wire.TxOut{Value: 5000000000, PkScript: append([]byte{0x00, 0x14}, make([]byte, 20)...)}
	chainParams := &chaincfg.RegressionNetParams

	setupBlock := func(ctx context.Context, db *sql.DB) {
		if err := InsertBlockHeader(ctx, db, &BlockHeader{
			Hash:       validBlockHash,
			Version:    1,
			PrevHash:   make([]byte, 32),
			MerkleRoot: make([]byte, 32),
			Timestamp:  time.Now().UTC().Truncate(time.Microsecond),
			Bits:       0x1d00ffff,
			Nonce:      2083236893,
		}); err != nil {
			t.Fatalf("setup: InsertBlockHeader failed: %v", err)
		}
	}

	tests := []struct {
		name        string
		setup       func(ctx context.Context, db *sql.DB)
		blockHash   []byte
		txHash      []byte
		index       int
		txOut       *wire.TxOut
		cancelCtx   bool
		wantErrCode pq.ErrorCode
	}{
		{
			name:      "valid insert succeeds",
			setup:     setupBlock,
			blockHash: validBlockHash,
			txHash:    validTxHash,
			index:     0,
			txOut:     validTxOut,
		},
		{
			name: "duplicate is silently ignored",
			setup: func(ctx context.Context, db *sql.DB) {
				setupBlock(ctx, db)
				if err := insertTxOut(ctx, db, validBlockHash, validTxHash, 0, validTxOut, chainParams); err != nil {
					t.Fatalf("setup: first insert failed: %v", err)
				}
			},
			blockHash: validBlockHash,
			txHash:    validTxHash,
			index:     0,
			txOut:     validTxOut,
		},
		{
			name:        "nil blockHash is rejected",
			blockHash:   nil,
			txHash:      validTxHash,
			index:       0,
			txOut:       validTxOut,
			wantErrCode: pgNotNullViolation,
		},
		{
			name:        "nil txHash is rejected",
			setup:       setupBlock,
			blockHash:   validBlockHash,
			txHash:      nil,
			index:       0,
			txOut:       validTxOut,
			wantErrCode: pgNotNullViolation,
		},
		{
			name:      "non-standard pkscript stores empty addresses without error",
			setup:     setupBlock,
			blockHash: validBlockHash,
			txHash:    validTxHash,
			index:     0,
			txOut:     &wire.TxOut{Value: 5000000000, PkScript: []byte{0x51}},
		},
		{
			name:        "blockHash with no matching block header is rejected",
			blockHash:   []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			txHash:      validTxHash,
			index:       0,
			txOut:       validTxOut,
			wantErrCode: pgFKViolation,
		},
		{
			name:      "cancelled context returns error",
			setup:     setupBlock,
			blockHash: validBlockHash,
			txHash:    validTxHash,
			index:     0,
			txOut:     validTxOut,
			cancelCtx: true,
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

			ctx := t.Context()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			err = insertTxOut(ctx, db, tt.blockHash, tt.txHash, tt.index, tt.txOut, chainParams)

			if tt.cancelCtx {
				if err == nil {
					t.Error("expected error for cancelled context, got nil")
				}
				return
			}

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

func TestInsertTxOutAddressPrefix(t *testing.T) {
	// P2WPKH script (OP_0 <20-byte-hash>) produces a bech32 address whose
	// prefix depends on which network's chain params are used.
	pkScript := append([]byte{0x00, 0x14}, make([]byte, 20)...)
	txOut := &wire.TxOut{Value: 5000000000, PkScript: pkScript}

	blockHash := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	txHash := []byte("tttttttttttttttttttttttttttttttt")

	tests := []struct {
		name           string
		chainParams    *chaincfg.Params
		wantAddrPrefix string
	}{
		{
			name:           "mainnet uses bc1q prefix",
			chainParams:    &chaincfg.MainNetParams,
			wantAddrPrefix: "bc1q",
		},
		{
			name:           "testnet3 uses tb1q prefix",
			chainParams:    &chaincfg.TestNet3Params,
			wantAddrPrefix: "tb1q",
		},
		{
			name:           "testnet4 uses tb1q prefix",
			chainParams:    &chaincfg.TestNet4Params,
			wantAddrPrefix: "tb1q",
		},
		{
			name:           "regtest uses bcrt1q prefix",
			chainParams:    &chaincfg.RegressionNetParams,
			wantAddrPrefix: "bcrt1q",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, drop, err := database.CreateNewRandomDatabase(t.Context())
			if err != nil {
				t.Fatalf("could not create test database: %s", err)
			}
			defer drop()

			if err := InsertBlockHeader(t.Context(), db, &BlockHeader{
				Hash:       blockHash,
				Version:    1,
				PrevHash:   make([]byte, 32),
				MerkleRoot: make([]byte, 32),
				Timestamp:  time.Now().UTC().Truncate(time.Microsecond),
				Bits:       0x1d00ffff,
				Nonce:      2083236893,
			}); err != nil {
				t.Fatalf("InsertBlockHeader: %v", err)
			}

			if err := insertTxOut(t.Context(), db, blockHash, txHash, 0, txOut, tt.chainParams); err != nil {
				t.Fatalf("insertTxOut: %v", err)
			}

			var addresses pq.StringArray
			if err := db.QueryRowContext(t.Context(),
				`SELECT addresses FROM txouts WHERE tx_hash = $1 AND tx_index = 0`, txHash,
			).Scan(&addresses); err != nil {
				t.Fatalf("querying addresses: %v", err)
			}

			if len(addresses) == 0 {
				t.Fatal("expected at least one address, got none")
			}
			if !strings.HasPrefix(addresses[0], tt.wantAddrPrefix) {
				t.Errorf("address = %q, want prefix %q", addresses[0], tt.wantAddrPrefix)
			}
		})
	}
}

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
