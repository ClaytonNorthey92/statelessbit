package statelessbit

import (
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"sync"
)

var ErrBlockExists = errors.New("block already exists")
var ErrBlockNotFound = errors.New("block not found")

type Store interface {
	InsertBlock(*wire.MsgBlock) error
	GetBlock(*chainhash.Hash) (*wire.MsgBlock, error)
	Clear() error
	ListUnspent(btcutil.Address, chainhash.Hash) ([]*wire.TxOut, error)
}

type BlockHandler struct {
	store Store
}

// should only be used for testing, blocks are stored with pointers so
// you will modify the stored values if you modify what is retrieved
type MemoryStore struct {
	mtx   sync.Mutex
	store map[string]*wire.MsgBlock
}

func NewBlockHandler(s Store) *BlockHandler {
	return &BlockHandler{
		store: s,
	}
}

func (bh *BlockHandler) StoreBlock(block *wire.MsgBlock) error {
	if err := bh.store.InsertBlock(block); err != nil {
		return err
	}

	return nil
}

func (bh *BlockHandler) GetBlock(hash *chainhash.Hash) (*wire.MsgBlock, error) {
	block, err := bh.store.GetBlock(hash)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func (bh *BlockHandler) ListUnspent(addr btcutil.Address, hash chainhash.Hash) ([]*wire.TxOut, error) {
	return bh.store.ListUnspent(addr, hash)
}

func (ms *MemoryStore) InsertBlock(block *wire.MsgBlock) error {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	if ms.store == nil {
		ms.store = map[string]*wire.MsgBlock{}
	}

	_, ok := ms.store[block.Header.BlockHash().String()]
	if ok {
		return fmt.Errorf("block already exists with hash %s: %w", block.Header.BlockHash().String(), ErrBlockExists)
	}

	ms.store[block.Header.BlockHash().String()] = block
	return nil
}

func (ms *MemoryStore) GetBlock(hash *chainhash.Hash) (*wire.MsgBlock, error) {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	block, ok := ms.store[hash.String()]
	if !ok {
		return nil, fmt.Errorf("could not find block with hash %s: %w", hash.String(), ErrBlockNotFound)
	}

	return block, nil
}

func (ms *MemoryStore) Clear() error {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	clear(ms.store)
	return nil
}

func keyForTxOut(txhash chainhash.Hash, index int) string {
	return fmt.Sprintf("txout:%s:%d", txhash, index)
}

func (ms *MemoryStore) ListUnspent(addr btcutil.Address, hash chainhash.Hash) ([]*wire.TxOut, error) {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	fmt.Printf("checking from block %s\n", hash)

	result := map[string]*wire.TxOut{}

	// brute force for testing
	for _, block := range ms.store {
		for _, tx := range block.Transactions {
			for i := range tx.TxOut {
				fmt.Printf("adding: %s\n", keyForTxOut(tx.TxHash(), i))
				result[keyForTxOut(tx.TxHash(), i)] = tx.TxOut[i]
			}
		}
	}

	for _, block := range ms.store {
		blockHash := block.Header.BlockHash()
		if blockHash.IsEqual(&hash) {
			for {
				for _, tx := range block.Transactions {
					for _, txin := range tx.TxIn {
						outpoint := txin.PreviousOutPoint
						fmt.Printf("deleting: %s\n", keyForTxOut(outpoint.Hash, int(outpoint.Index)))
						delete(result, keyForTxOut(outpoint.Hash, int(outpoint.Index)))
					}
				}

				if block.Header.PrevBlock == (chainhash.Hash{}) {
					break
				}

				var err error
				ms.mtx.Unlock()
				block, err = ms.GetBlock(&block.Header.PrevBlock)
				ms.mtx.Lock()
				if err != nil {
					return nil, err
				}
			}

			break
		}

	}

	toReturn := []*wire.TxOut{}

	for _, r := range result {
		toReturn = append(toReturn, r)
	}

	return toReturn, nil
}
