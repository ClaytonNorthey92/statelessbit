package statelessbit

import (
	"context"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/go-test/deep"
	mydatabase "mydatabase"
)

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
							TxOut: []*wire.TxOut{
								&wire.TxOut{},
							},
						},
					},
				},
				&Block100000,
				&wire.MsgBlock{
					Header: wire.BlockHeader{
						PrevBlock: (chainhash.Hash{}),
						Timestamp: time.Unix(1, 0),
					},
					Transactions: []*wire.MsgTx{
						&wire.MsgTx{
							TxOut: []*wire.TxOut{
								&wire.TxOut{},
							},
						},
					},
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

		db, _, err := mydatabase.CreateNewRandomDatabase(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		// defer dropFn()

		for _, s := range []Store{
			&MemoryStore{},
			NewPostgresStore(db, &chaincfg.SimNetParams),
		} {
			t.Run(tti.name, func(t *testing.T) {
				bh := NewBlockHandler(s)
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
}
