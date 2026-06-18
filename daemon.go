package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
)

type Daemon struct {
	db     *sql.DB
	client *rpcclient.Client
}

func NewDaemon(db *sql.DB, client *rpcclient.Client) *Daemon {
	return &Daemon{db: db, client: client}
}

func (d *Daemon) Run(ctx context.Context) error {
	if err := d.sync(ctx); err != nil {
		log.Printf("sync error: %v", err)
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := d.sync(ctx); err != nil {
				log.Printf("sync error: %v", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (d *Daemon) sync(ctx context.Context) error {
	tips, err := d.client.GetChainTips()
	if err != nil {
		return fmt.Errorf("getting chain tips: %w", err)
	}

	var activeTipHash string
	for _, tip := range tips {
		if tip.Status == "active" {
			activeTipHash = tip.Hash
			break
		}
	}

	if activeTipHash == "" {
		return fmt.Errorf("no active tip found")
	}

	hash, err := chainhash.NewHashFromStr(activeTipHash)
	if err != nil {
		return fmt.Errorf("parsing active tip hash %q: %w", activeTipHash, err)
	}

	log.Printf("syncing from active tip %s", activeTipHash)
	return d.syncFrom(ctx, *hash)
}

func (d *Daemon) syncFrom(ctx context.Context, tip chainhash.Hash) error {
	current := tip
	inserted := 0

	for {
		exists, err := d.blockExists(ctx, current)
		if err != nil {
			return err
		}
		if exists {
			break
		}

		block, err := d.client.GetBlock(&current)
		if err != nil {
			return fmt.Errorf("fetching block %s: %w", &current, err)
		}

		if err := InsertMsgBlock(ctx, d.db, block); err != nil {
			return fmt.Errorf("inserting block %s: %w", &current, err)
		}

		inserted++
		log.Printf("inserted block %s (%d so far this sync)", &current, inserted)

		prev := block.Header.PrevBlock
		if prev == (chainhash.Hash{}) {
			break // reached genesis
		}
		current = prev
	}

	if inserted > 0 {
		log.Printf("sync complete: inserted %d block(s)", inserted)
	}
	return nil
}

func (d *Daemon) blockExists(ctx context.Context, hash chainhash.Hash) (bool, error) {
	var exists bool
	err := d.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM block_headers WHERE hash = $1)`,
		hash[:],
	).Scan(&exists)
	return exists, err
}
