package statelessbit

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
)

type ChainTracker struct {
	client *rpcclient.Client
}

func NewChainTracker(client *rpcclient.Client) (*ChainTracker, error) {
	return &ChainTracker{
		client: client,
	}, nil
}

func (c *ChainTracker) forEachRawBlock(callback func(*wire.MsgBlock) error) error {
	chainTips, err := c.client.GetChainTips()
	if err != nil {
		return err
	}

	for _, chainTip := range chainTips {
		if chainTip.Status != "active" {
			continue
		}
		
		thisHash, err := chainhash.NewHashFromStr(chainTip.Hash)
		if err != nil {
			return err
		}

		for {
			block, err := c.client.GetBlock(thisHash)
			if err != nil {
				return err
			}

			if err := callback(block); err != nil {
				return err
			}

			// Clayton note: double check this will abort
			if block.Header.PrevBlock == (chainhash.Hash{}) {
				break
			}

			thisHash = &block.Header.PrevBlock
		}
	}

	return nil
}

