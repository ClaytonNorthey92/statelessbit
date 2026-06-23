package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lib/pq"
	"mydatabase"
)

func testTxIn() *wire.TxIn {
	return &wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0,
		},
		SignatureScript: []byte{0x01},
		Sequence:        wire.MaxTxInSequenceNum,
	}
}

func testTxOut() *wire.TxOut {
	// P2WPKH script (OP_0 <20-byte-hash>) so address extraction succeeds.
	return &wire.TxOut{
		Value:    5000000000,
		PkScript: append([]byte{0x00, 0x14}, make([]byte, 20)...),
	}
}

func newTestMsgBlock(txins []*wire.TxIn, txouts []*wire.TxOut) *wire.MsgBlock {
	return &wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   1,
			Timestamp: time.Unix(1231006505, 0),
			Bits:      0x1d00ffff,
			Nonce:     2083236893,
		},
		Transactions: []*wire.MsgTx{
			{
				Version: 1,
				TxIn:    txins,
				TxOut:   txouts,
			},
		},
	}
}

func assertBlockHeader(ctx context.Context, t *testing.T, db *sql.DB, msg *wire.MsgBlock) {
	t.Helper()

	blockHash := msg.Header.BlockHash()
	prevHash := msg.Header.PrevBlock
	merkleRoot := msg.Header.MerkleRoot

	var gotHash, gotPrevHash, gotMerkleRoot []byte
	var gotVersion int32
	var gotTimestamp time.Time
	var gotBits, gotNonce int64

	err := db.QueryRowContext(ctx, `
		SELECT hash, version, prev_hash, merkle_root, timestamp, bits, nonce
		FROM block_headers
		WHERE hash = $1`, blockHash[:]).
		Scan(&gotHash, &gotVersion, &gotPrevHash, &gotMerkleRoot, &gotTimestamp, &gotBits, &gotNonce)
	if err != nil {
		t.Fatalf("querying block_headers: %v", err)
	}

	t.Logf("hash: got=%s, want=%s", hex.EncodeToString(gotHash), hex.EncodeToString(blockHash[:]))
	if !bytes.Equal(gotHash, blockHash[:]) {
		t.Errorf("hash = %s, want %s", hex.EncodeToString(gotHash), hex.EncodeToString(blockHash[:]))
	}
	if gotVersion != msg.Header.Version {
		t.Errorf("version = %d, want %d", gotVersion, msg.Header.Version)
	}
	t.Logf("prev_hash: got=%s, want=%s", hex.EncodeToString(gotPrevHash), hex.EncodeToString(prevHash[:]))
	if !bytes.Equal(gotPrevHash, prevHash[:]) {
		t.Errorf("prev_hash = %s, want %s", hex.EncodeToString(gotPrevHash), hex.EncodeToString(prevHash[:]))
	}
	t.Logf("merkle_root: got=%s, want=%s", hex.EncodeToString(gotMerkleRoot), hex.EncodeToString(merkleRoot[:]))
	if !bytes.Equal(gotMerkleRoot, merkleRoot[:]) {
		t.Errorf("merkle_root = %s, want %s", hex.EncodeToString(gotMerkleRoot), hex.EncodeToString(merkleRoot[:]))
	}
	if !gotTimestamp.UTC().Equal(msg.Header.Timestamp.UTC()) {
		t.Errorf("timestamp = %v, want %v", gotTimestamp.UTC(), msg.Header.Timestamp.UTC())
	}
	if gotBits != int64(msg.Header.Bits) {
		t.Errorf("bits = %d, want %d", gotBits, msg.Header.Bits)
	}
	if gotNonce != int64(msg.Header.Nonce) {
		t.Errorf("nonce = %d, want %d", gotNonce, msg.Header.Nonce)
	}
}

func assertTxOuts(ctx context.Context, t *testing.T, db *sql.DB, blockHash []byte, tx *wire.MsgTx, chainParams *chaincfg.Params) {
	t.Helper()

	txHash := tx.TxHash()

	rows, err := db.QueryContext(ctx, `
		SELECT block_hash, tx_hash, tx_index, value, addresses
		FROM txouts
		WHERE block_hash = $1
		ORDER BY tx_index`, blockHash)
	if err != nil {
		t.Fatalf("querying txouts: %v", err)
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		if i >= len(tx.TxOut) {
			t.Errorf("got more txout rows than expected (%d)", len(tx.TxOut))
			break
		}

		var gotBlockHash, gotTxHash []byte
		var gotTxIndex int
		var gotValue int64
		var gotAddresses pq.StringArray

		if err := rows.Scan(&gotBlockHash, &gotTxHash, &gotTxIndex, &gotValue, &gotAddresses); err != nil {
			t.Fatalf("scanning txout row %d: %v", i, err)
		}

		want := tx.TxOut[i]
		_, wantAddrs, _, _ := txscript.ExtractPkScriptAddrs(want.PkScript, chainParams)
		wantAddresses := make(pq.StringArray, len(wantAddrs))
		for j, addr := range wantAddrs {
			wantAddresses[j] = addr.EncodeAddress()
		}

		t.Logf("txout[%d] block_hash: got=%s, want=%s", i, hex.EncodeToString(gotBlockHash), hex.EncodeToString(blockHash))
		if !bytes.Equal(gotBlockHash, blockHash) {
			t.Errorf("txout[%d] block_hash = %s, want %s", i, hex.EncodeToString(gotBlockHash), hex.EncodeToString(blockHash))
		}
		t.Logf("txout[%d] tx_hash: got=%s, want=%s", i, hex.EncodeToString(gotTxHash), hex.EncodeToString(txHash[:]))
		if !bytes.Equal(gotTxHash, txHash[:]) {
			t.Errorf("txout[%d] tx_hash = %s, want %s", i, hex.EncodeToString(gotTxHash), hex.EncodeToString(txHash[:]))
		}
		if gotTxIndex != i {
			t.Errorf("txout[%d] tx_index = %d, want %d", i, gotTxIndex, i)
		}
		if gotValue != want.Value {
			t.Errorf("txout[%d] value = %d, want %d", i, gotValue, want.Value)
		}
		t.Logf("txout[%d] addresses: got=%v, want=%v", i, []string(gotAddresses), []string(wantAddresses))
		if len(gotAddresses) != len(wantAddresses) {
			t.Errorf("txout[%d] addresses = %v, want %v", i, []string(gotAddresses), []string(wantAddresses))
		} else {
			for j := range gotAddresses {
				if gotAddresses[j] != wantAddresses[j] {
					t.Errorf("txout[%d] addresses[%d] = %q, want %q", i, j, gotAddresses[j], wantAddresses[j])
				}
			}
		}

		i++
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("iterating txout rows: %v", err)
	}
	if i != len(tx.TxOut) {
		t.Errorf("txout count = %d, want %d", i, len(tx.TxOut))
	}
}

func assertTxIns(ctx context.Context, t *testing.T, db *sql.DB, blockHash []byte, tx *wire.MsgTx) {
	t.Helper()

	txHash := tx.TxHash()

	rows, err := db.QueryContext(ctx, `
		SELECT block_hash, tx_hash, tx_index, prev_out_hash, prev_out_index, script_sig, sequence
		FROM txins
		WHERE block_hash = $1
		ORDER BY tx_index`, blockHash)
	if err != nil {
		t.Fatalf("querying txins: %v", err)
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		if i >= len(tx.TxIn) {
			t.Errorf("got more txin rows than expected (%d)", len(tx.TxIn))
			break
		}

		var gotBlockHash, gotTxHash, gotPrevOutHash, gotScriptSig []byte
		var gotTxIndex int
		var gotPrevOutIndex, gotSequence int64

		if err := rows.Scan(&gotBlockHash, &gotTxHash, &gotTxIndex, &gotPrevOutHash, &gotPrevOutIndex, &gotScriptSig, &gotSequence); err != nil {
			t.Fatalf("scanning txin row %d: %v", i, err)
		}

		want := tx.TxIn[i]
		wantPrevOutHash := want.PreviousOutPoint.Hash

		t.Logf("txin[%d] block_hash: got=%s, want=%s", i, hex.EncodeToString(gotBlockHash), hex.EncodeToString(blockHash))
		if !bytes.Equal(gotBlockHash, blockHash) {
			t.Errorf("txin[%d] block_hash = %s, want %s", i, hex.EncodeToString(gotBlockHash), hex.EncodeToString(blockHash))
		}
		t.Logf("txin[%d] tx_hash: got=%s, want=%s", i, hex.EncodeToString(gotTxHash), hex.EncodeToString(txHash[:]))
		if !bytes.Equal(gotTxHash, txHash[:]) {
			t.Errorf("txin[%d] tx_hash = %s, want %s", i, hex.EncodeToString(gotTxHash), hex.EncodeToString(txHash[:]))
		}
		if gotTxIndex != i {
			t.Errorf("txin[%d] tx_index = %d, want %d", i, gotTxIndex, i)
		}
		t.Logf("txin[%d] prev_out_hash: got=%s, want=%s", i, hex.EncodeToString(gotPrevOutHash), hex.EncodeToString(wantPrevOutHash[:]))
		if !bytes.Equal(gotPrevOutHash, wantPrevOutHash[:]) {
			t.Errorf("txin[%d] prev_out_hash = %s, want %s", i, hex.EncodeToString(gotPrevOutHash), hex.EncodeToString(wantPrevOutHash[:]))
		}
		if gotPrevOutIndex != int64(want.PreviousOutPoint.Index) {
			t.Errorf("txin[%d] prev_out_index = %d, want %d", i, gotPrevOutIndex, want.PreviousOutPoint.Index)
		}
		t.Logf("txin[%d] script_sig: got=%s, want=%s", i, hex.EncodeToString(gotScriptSig), hex.EncodeToString(want.SignatureScript))
		if !bytes.Equal(gotScriptSig, want.SignatureScript) {
			t.Errorf("txin[%d] script_sig = %s, want %s", i, hex.EncodeToString(gotScriptSig), hex.EncodeToString(want.SignatureScript))
		}
		if gotSequence != int64(want.Sequence) {
			t.Errorf("txin[%d] sequence = %d, want %d", i, gotSequence, want.Sequence)
		}

		i++
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("iterating txin rows: %v", err)
	}
	if i != len(tx.TxIn) {
		t.Errorf("txin count = %d, want %d", i, len(tx.TxIn))
	}
}

func TestInsertMsgBlock(t *testing.T) {
	tests := []struct {
		name   string
		txins  []*wire.TxIn
		txouts []*wire.TxOut
	}{
		{
			name:   "empty txouts",
			txins:  []*wire.TxIn{testTxIn()},
			txouts: []*wire.TxOut{},
		},
		{
			name:   "1 txout",
			txins:  []*wire.TxIn{testTxIn()},
			txouts: []*wire.TxOut{testTxOut()},
		},
		{
			name:   "multiple txouts",
			txins:  []*wire.TxIn{testTxIn()},
			txouts: []*wire.TxOut{testTxOut(), testTxOut(), testTxOut()},
		},
		{
			name:   "empty txins",
			txins:  []*wire.TxIn{},
			txouts: []*wire.TxOut{testTxOut()},
		},
		{
			name:   "1 txin",
			txins:  []*wire.TxIn{testTxIn()},
			txouts: []*wire.TxOut{testTxOut()},
		},
		{
			name:   "multiple txins",
			txins:  []*wire.TxIn{testTxIn(), testTxIn(), testTxIn()},
			txouts: []*wire.TxOut{testTxOut()},
		},
	}

	chainParams := &chaincfg.RegressionNetParams

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, drop, err := database.CreateNewRandomDatabase(t.Context())
			if err != nil {
				t.Fatalf("could not create test database: %s", err)
			}
			defer drop()

			msg := newTestMsgBlock(tt.txins, tt.txouts)

			if err := InsertMsgBlock(t.Context(), db, msg, chainParams); err != nil {
				t.Fatalf("InsertMsgBlock() error = %v", err)
			}

			blockHash := msg.Header.BlockHash()
			assertBlockHeader(t.Context(), t, db, msg)
			for _, tx := range msg.Transactions {
				assertTxOuts(t.Context(), t, db, blockHash[:], tx, chainParams)
				assertTxIns(t.Context(), t, db, blockHash[:], tx)
			}
		})
	}
}
