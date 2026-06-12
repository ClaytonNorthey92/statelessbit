package statelessbit

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/go-test/deep"
)

func TestStoreBlock(t *testing.T) {
	var memoryStore MemoryStore

	stores := []Store{&memoryStore}

	for i, s := range stores {

		t.Run(fmt.Sprintf("testing store/retrieve at index %d", i), func(t *testing.T) {
			if err := s.Clear(); err != nil {
				t.Fatal(err)
			}

			bh := NewBlockHandler(s)

			if err := bh.StoreBlock(&Block100000); err != nil {
				t.Fatal(err)
			}

			hash := Block100000.Header.BlockHash()
			storedBlock, err := bh.GetBlock(&hash)
			if err != nil {
				t.Fatal(err)
			}

			if diff := deep.Equal(storedBlock, &Block100000); len(diff) > 0 {
				t.Fatalf("unexpected diff: %s", diff)
			}
		})

		t.Run(fmt.Sprintf("do not allow duplicates at index %d", i), func(t *testing.T) {
			if err := s.Clear(); err != nil {
				t.Fatal(err)
			}

			bh := NewBlockHandler(s)

			if err := bh.StoreBlock(&Block100000); err != nil {
				t.Fatal(err)
			}

			err := bh.StoreBlock(&Block100000)
			if err == nil {
				t.Fatal("expected error")
			}

			if !errors.Is(err, ErrBlockExists) {
				t.Fatalf("unexpected error: %s", err)
			}
		})

		t.Run(fmt.Sprintf("block not found at index %d", i), func(t *testing.T) {
			if err := s.Clear(); err != nil {
				t.Fatal(err)
			}

			bh := NewBlockHandler(s)

			hash := Block100000.Header.BlockHash()

			_, err := bh.GetBlock(&hash)

			if err == nil {
				t.Fatal("expected error")
			}

			if !errors.Is(err, ErrBlockNotFound) {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}

func TestIndexUTXOs(t *testing.T) {
	type testTableItem struct {
		name  string
		chain []*wire.MsgBlock
	}

	testTable := []testTableItem{
		testTableItem{
			name: "multiple blocks",
			chain: []*wire.MsgBlock{
				&wire.MsgBlock{
					Header: wire.BlockHeader{
						PrevBlock: (chainhash.Hash{}),
						Timestamp: time.Unix(3, 0),
					},
					Transactions: []*wire.MsgTx{
						&wire.MsgTx{
							TxIn: []*wire.TxIn{
								&wire.TxIn{
									PreviousOutPoint: wire.OutPoint{
										Index: 1,
										Hash:  Block100000.Transactions[1].TxHash(),
									},
									SignatureScript: []byte{},
								},
							},
							TxOut: []*wire.TxOut{},
						},
					},
				},
				&Block100000,
				&wire.MsgBlock{
					Header: wire.BlockHeader{
						PrevBlock: (chainhash.Hash{}),
						Timestamp: time.Unix(1, 0),
					},
					Transactions: []*wire.MsgTx{},
				},
			},
		},
	}

	for _, tti := range testTable {
		// setup chain links
		for i := len(tti.chain) - 2; i >= 0; i-- {
			t.Logf("setting PrevBlock of %s to %s", tti.chain[i].Header.BlockHash(), tti.chain[i+1].Header.BlockHash())
			tti.chain[i].Header.PrevBlock = tti.chain[i+1].Header.BlockHash()
			t.Logf("the new block hash is %s", tti.chain[i].Header.BlockHash())
		}

		// log for double-checking
		for i := range tti.chain {
			if i == len(tti.chain)-1 {
				break
			}

			t.Logf("%s --> %s", tti.chain[i].Header.BlockHash(), tti.chain[i+1].Header.BlockHash())
		}

		t.Run(tti.name, func(t *testing.T) {
			bh := NewBlockHandler(&MemoryStore{})
			for _, block := range tti.chain {
				if err := bh.StoreBlock(block); err != nil {
					t.Fatal(err)
				}
			}

			unspentInQuestion := tti.chain[1].Transactions[1].TxOut[1]

			_, addrs, _, err := txscript.ExtractPkScriptAddrs(unspentInQuestion.PkScript, &chaincfg.SimNetParams)
			if err != nil {
				t.Fatalf("Error extracting address: %v\n", err)
			}

			// from tip, we should NOT find the txin in the last block as unspent
			txouts, err := bh.ListUnspent(addrs[0], tti.chain[0].Header.BlockHash())
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("checking that we do not have utxo")
			for _, txout := range txouts {
				if diff := deep.Equal(unspentInQuestion, txout); len(diff) == 0 {
					t.Fatal("found txout when we should not have")
				}
			}

			// from tip-1, we should find the txin in the last block as unspent
			txouts, err = bh.ListUnspent(addrs[0], tti.chain[1].Header.BlockHash())
			if err != nil {
				t.Fatal(err)
			}

			found := false
			for _, txout := range txouts {
				if diff := deep.Equal(unspentInQuestion, txout); len(diff) == 0 {
					found = true
				}
			}

			if !found {
				t.Fatal("should have found utxo")
			}
		})
	}
}
