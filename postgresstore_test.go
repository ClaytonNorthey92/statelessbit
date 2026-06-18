package statelessbit

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	mydatabase "mydatabase"
)

func countTxoutsForBlock(t *testing.T, db *sql.DB, blockHash string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM txouts WHERE created_at_block_hash = $1", blockHash).Scan(&count); err != nil {
		t.Fatalf("countTxoutsForBlock: %v", err)
	}
	return count
}

func countTxoutSpendsForBlock(t *testing.T, db *sql.DB, blockHash string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM txout_spends WHERE spent_at_block_hash = $1", blockHash).Scan(&count); err != nil {
		t.Fatalf("countTxoutSpendsForBlock: %v", err)
	}
	return count
}

func makeCoinbaseTx(sigScript []byte, outputs ...*wire.TxOut) *wire.MsgTx {
	return &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{Hash: chainhash.Hash{}, Index: 0xffffffff},
			SignatureScript:  sigScript,
			Sequence:         0xffffffff,
		}},
		TxOut: outputs,
	}
}

func makeSpendTx(prevHash chainhash.Hash, prevIndex uint32) *wire.MsgTx {
	return &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{Hash: prevHash, Index: prevIndex},
			SignatureScript:  []byte{},
			Sequence:         0xffffffff,
		}},
		TxOut: []*wire.TxOut{{Value: 500, PkScript: []byte{}}},
	}
}

func assertBlockActive(t *testing.T, db *sql.DB, blockHash string, expected bool) {
	t.Helper()
	var total, matching int
	err := db.QueryRow(
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE active_block = $2) FROM txouts WHERE created_at_block_hash = $1`,
		blockHash, expected,
	).Scan(&total, &matching)
	if err != nil {
		t.Fatalf("assertBlockActive: %v", err)
	}
	if total == 0 {
		t.Fatalf("assertBlockActive: no txouts found for block %s", blockHash)
	}
	if matching != total {
		t.Errorf("block %s: expected all %d txout(s) to have active_block=%v, but only %d match", blockHash, total, expected, matching)
	}
}

func TestSetActiveFromTip_SetsActiveChainTrue(t *testing.T) {
	db, dropFn, err := mydatabase.CreateNewRandomDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer dropFn()

	store := NewPostgresStore(db, &chaincfg.SimNetParams)

	blockGenesis := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: chainhash.Hash{},
			Timestamp: time.Unix(1, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x01}, &wire.TxOut{Value: 1000, PkScript: []byte{}}),
		},
	}
	blockA := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: blockGenesis.Header.BlockHash(),
			Timestamp: time.Unix(2, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x02}, &wire.TxOut{Value: 2000, PkScript: []byte{}}),
		},
	}
	blockB := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: blockA.Header.BlockHash(),
			Timestamp: time.Unix(3, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x03}, &wire.TxOut{Value: 3000, PkScript: []byte{}}),
		},
	}

	for _, block := range []*wire.MsgBlock{blockGenesis, blockA, blockB} {
		if err := store.InsertBlock(block); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	if err := store.SetActiveFromTip(blockB.Header.BlockHash()); err != nil {
		t.Fatalf("SetActiveFromTip: %v", err)
	}

	assertBlockActive(t, db, blockGenesis.Header.BlockHash().String(), true)
	assertBlockActive(t, db, blockA.Header.BlockHash().String(), true)
	assertBlockActive(t, db, blockB.Header.BlockHash().String(), true)
}

func TestSetActiveFromTip_ClearsBlocksNotOnTipChain(t *testing.T) {
	// Chain structure:
	//   genesis → blockA → blockB  (original tip)
	//                    ↘ blockC  (reorg tip)
	//
	// After reorging to blockC, blockB must be inactive.

	db, dropFn, err := mydatabase.CreateNewRandomDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer dropFn()

	store := NewPostgresStore(db, &chaincfg.SimNetParams)

	blockGenesis := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: chainhash.Hash{},
			Timestamp: time.Unix(1, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x01}, &wire.TxOut{Value: 1000, PkScript: []byte{}}),
		},
	}
	blockA := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: blockGenesis.Header.BlockHash(),
			Timestamp: time.Unix(2, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x02}, &wire.TxOut{Value: 2000, PkScript: []byte{}}),
		},
	}
	// blockB and blockC both fork from blockA
	blockB := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: blockA.Header.BlockHash(),
			Timestamp: time.Unix(3, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x03}, &wire.TxOut{Value: 3000, PkScript: []byte{}}),
		},
	}
	blockC := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: blockA.Header.BlockHash(),
			Timestamp: time.Unix(4, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x04}, &wire.TxOut{Value: 4000, PkScript: []byte{}}),
		},
	}

	for _, block := range []*wire.MsgBlock{blockGenesis, blockA, blockB, blockC} {
		if err := store.InsertBlock(block); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	// Set the original tip to blockB, then reorg to blockC.
	if err := store.SetActiveFromTip(blockB.Header.BlockHash()); err != nil {
		t.Fatalf("SetActiveFromTip blockB: %v", err)
	}
	if err := store.SetActiveFromTip(blockC.Header.BlockHash()); err != nil {
		t.Fatalf("SetActiveFromTip blockC: %v", err)
	}

	// Blocks on the new active chain must be true.
	assertBlockActive(t, db, blockGenesis.Header.BlockHash().String(), true)
	assertBlockActive(t, db, blockA.Header.BlockHash().String(), true)
	assertBlockActive(t, db, blockC.Header.BlockHash().String(), true)

	// blockB is no longer connected to the tip and must be false.
	assertBlockActive(t, db, blockB.Header.BlockHash().String(), false)
}

func TestDeleteBlock_DeletesTxouts(t *testing.T) {
	db, dropFn, err := mydatabase.CreateNewRandomDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer dropFn()

	store := NewPostgresStore(db, &chaincfg.SimNetParams)

	blockA := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: chainhash.Hash{},
			Timestamp: time.Unix(1, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x01},
				&wire.TxOut{Value: 1000, PkScript: []byte{}},
				&wire.TxOut{Value: 2000, PkScript: []byte{}},
			),
		},
	}
	blockB := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: blockA.Header.BlockHash(),
			Timestamp: time.Unix(2, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x02},
				&wire.TxOut{Value: 3000, PkScript: []byte{}},
			),
		},
	}

	if err := store.InsertBlock(blockA); err != nil {
		t.Fatalf("InsertBlock blockA: %v", err)
	}
	if err := store.InsertBlock(blockB); err != nil {
		t.Fatalf("InsertBlock blockB: %v", err)
	}

	blockAHash := blockA.Header.BlockHash()
	blockBHash := blockB.Header.BlockHash()

	if got := countTxoutsForBlock(t, db, blockAHash.String()); got != 2 {
		t.Fatalf("pre-delete: expected 2 txouts for blockA, got %d", got)
	}
	if got := countTxoutsForBlock(t, db, blockBHash.String()); got != 1 {
		t.Fatalf("pre-delete: expected 1 txout for blockB, got %d", got)
	}

	if err := store.DeleteBlock(blockAHash); err != nil {
		t.Fatalf("DeleteBlock: %v", err)
	}

	if got := countTxoutsForBlock(t, db, blockAHash.String()); got != 0 {
		t.Fatalf("post-delete: expected 0 txouts for blockA, got %d", got)
	}
	if got := countTxoutsForBlock(t, db, blockBHash.String()); got != 1 {
		t.Fatalf("post-delete: expected blockB txout untouched (1), got %d", got)
	}
}

func TestDeleteBlock_DeletesTxoutSpends(t *testing.T) {
	db, dropFn, err := mydatabase.CreateNewRandomDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer dropFn()

	store := NewPostgresStore(db, &chaincfg.SimNetParams)

	coinbaseTxA := makeCoinbaseTx([]byte{0x01}, &wire.TxOut{Value: 1000, PkScript: []byte{}})
	blockA := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: chainhash.Hash{},
			Timestamp: time.Unix(1, 0),
		},
		Transactions: []*wire.MsgTx{coinbaseTxA},
	}

	if err := store.InsertBlock(blockA); err != nil {
		t.Fatalf("InsertBlock blockA: %v", err)
	}

	blockB := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: blockA.Header.BlockHash(),
			Timestamp: time.Unix(2, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x02}, &wire.TxOut{Value: 2000, PkScript: []byte{}}),
			makeSpendTx(coinbaseTxA.TxHash(), 0),
		},
	}

	if err := store.InsertBlock(blockB); err != nil {
		t.Fatalf("InsertBlock blockB: %v", err)
	}

	blockBHash := blockB.Header.BlockHash()

	if got := countTxoutSpendsForBlock(t, db, blockBHash.String()); got != 1 {
		t.Fatalf("pre-delete: expected 1 txout_spend for blockB, got %d", got)
	}

	if err := store.DeleteBlock(blockBHash); err != nil {
		t.Fatalf("DeleteBlock: %v", err)
	}

	if got := countTxoutSpendsForBlock(t, db, blockBHash.String()); got != 0 {
		t.Fatalf("post-delete: expected 0 txout_spends for blockB, got %d", got)
	}
}

func TestDeleteBlock_DoesNotDeleteNonMatchingRecords(t *testing.T) {
	db, dropFn, err := mydatabase.CreateNewRandomDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer dropFn()

	store := NewPostgresStore(db, &chaincfg.SimNetParams)

	coinbaseTxA := makeCoinbaseTx([]byte{0x01},
		&wire.TxOut{Value: 1000, PkScript: []byte{}},
		&wire.TxOut{Value: 2000, PkScript: []byte{}},
	)
	blockA := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: chainhash.Hash{},
			Timestamp: time.Unix(1, 0),
		},
		Transactions: []*wire.MsgTx{coinbaseTxA},
	}

	blockB := &wire.MsgBlock{
		Header: wire.BlockHeader{
			PrevBlock: blockA.Header.BlockHash(),
			Timestamp: time.Unix(2, 0),
		},
		Transactions: []*wire.MsgTx{
			makeCoinbaseTx([]byte{0x02}, &wire.TxOut{Value: 3000, PkScript: []byte{}}),
			makeSpendTx(coinbaseTxA.TxHash(), 0),
		},
	}

	if err := store.InsertBlock(blockA); err != nil {
		t.Fatalf("InsertBlock blockA: %v", err)
	}
	if err := store.InsertBlock(blockB); err != nil {
		t.Fatalf("InsertBlock blockB: %v", err)
	}

	blockAHash := blockA.Header.BlockHash()
	blockBHash := blockB.Header.BlockHash()

	// Deleting blockA should only remove txouts created in blockA.
	// txouts and txout_spends belonging to blockB must be untouched.
	if err := store.DeleteBlock(blockAHash); err != nil {
		t.Fatalf("DeleteBlock: %v", err)
	}

	// blockB has two txouts: one from the coinbase tx and one from the spend tx
	if got := countTxoutsForBlock(t, db, blockBHash.String()); got != 2 {
		t.Fatalf("expected blockB txouts untouched (2), got %d", got)
	}

	// blockB has one txout_spend (the spend of blockA's coinbase output)
	if got := countTxoutSpendsForBlock(t, db, blockBHash.String()); got != 1 {
		t.Fatalf("expected blockB txout_spends untouched (1), got %d", got)
	}
}
