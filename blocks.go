package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/btcsuite/btcd/wire"
)

type BlockHeader struct {
	Hash       []byte
	Version    int32
	PrevHash   []byte
	MerkleRoot []byte
	Timestamp  time.Time
	Bits       uint32
	Nonce      uint32
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func MsgBlockToBlockHeader(msg *wire.MsgBlock) *BlockHeader {
	hash := msg.Header.BlockHash()
	return &BlockHeader{
		Hash:       hash[:],
		Version:    msg.Header.Version,
		PrevHash:   msg.Header.PrevBlock[:],
		MerkleRoot: msg.Header.MerkleRoot[:],
		Timestamp:  msg.Header.Timestamp,
		Bits:       msg.Header.Bits,
		Nonce:      msg.Header.Nonce,
	}
}

func InsertBlockHeader(ctx context.Context, db *sql.DB, b *BlockHeader) error {
	return insertBlockHeader(ctx, db, b)
}

func UpdateBlockHeaderHeight(ctx context.Context, db *sql.DB, blockHash []byte, height int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE block_headers SET height = $1 WHERE hash = $2`,
		height, blockHash,
	)
	return err
}

func SetBlockHeights(ctx context.Context, db *sql.DB) error {
	var hasHeights bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM block_headers WHERE height > 0)`,
	).Scan(&hasHeights); err != nil {
		return fmt.Errorf("checking for existing heights: %w", err)
	}

	if hasHeights {
		return setBlockHeightsIncremental(ctx, db)
	}
	return setBlockHeightsFromGenesis(ctx, db)
}

func setBlockHeightsFromGenesis(ctx context.Context, db *sql.DB) error {
	var current []byte
	err := db.QueryRowContext(ctx,
		`SELECT hash FROM block_headers WHERE prev_hash = $1`,
		make([]byte, 32),
	).Scan(&current)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("finding genesis block: %w", err)
	}

	for height := int64(0); ; height++ {
		log.Printf("setting height %d for block %s", height, hex.EncodeToString(current))
		if err := UpdateBlockHeaderHeight(ctx, db, current, height); err != nil {
			return fmt.Errorf("setting height %d: %w", height, err)
		}
		log.Printf("done setting height")

		var next []byte
		err := db.QueryRowContext(ctx,
			`SELECT hash FROM block_headers WHERE prev_hash = $1`,
			current,
		).Scan(&next)
		if err == sql.ErrNoRows {
			log.Printf("set block heights complete at height %d", height)
			break
		}
		if err != nil {
			return fmt.Errorf("finding block at height %d: %w", height+1, err)
		}

		current = next
	}

	return nil
}

func setBlockHeightsIncremental(ctx context.Context, db *sql.DB) error {
	for {
		res, err := db.ExecContext(ctx, `
			UPDATE block_headers
			SET height = (
				SELECT bh2.height + 1
				FROM block_headers bh2
				WHERE bh2.hash = block_headers.prev_hash
			)
			WHERE height IS NULL
			AND EXISTS (
				SELECT 1 FROM block_headers bh2
				WHERE bh2.hash = block_headers.prev_hash
				AND bh2.height IS NOT NULL
			)
		`)
		if err != nil {
			return fmt.Errorf("incremental height update: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if n == 0 {
			log.Printf("incremental height update complete")
			break
		}
		log.Printf("set heights for %d blocks", n)
	}
	return nil
}

func InsertMsgBlock(ctx context.Context, db *sql.DB, msg *wire.MsgBlock) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	header := MsgBlockToBlockHeader(msg)
	if err := insertBlockHeader(ctx, tx, header); err != nil {
		return err
	}

	for _, msgTx := range msg.Transactions {
		txHash := msgTx.TxHash()
		for i, txOut := range msgTx.TxOut {
			if err := insertTxOut(ctx, tx, header.Hash, txHash[:], i, txOut); err != nil {
				return err
			}
		}
		for i, txIn := range msgTx.TxIn {
			if err := insertTxIn(ctx, tx, header.Hash, txHash[:], i, txIn); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func insertBlockHeader(ctx context.Context, ex sqlExecer, b *BlockHeader) error {
	_, err := ex.ExecContext(ctx, `
		INSERT INTO block_headers (hash, version, prev_hash, merkle_root, timestamp, bits, nonce)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		b.Hash, b.Version, b.PrevHash, b.MerkleRoot, b.Timestamp, int64(b.Bits), int64(b.Nonce),
	)
	return err
}

func insertTxIn(ctx context.Context, ex sqlExecer, blockHash, txHash []byte, index int, txIn *wire.TxIn) error {
	prevHash := txIn.PreviousOutPoint.Hash
	_, err := ex.ExecContext(ctx, `
		INSERT INTO txins (block_hash, tx_hash, tx_index, prev_out_hash, prev_out_index, script_sig, sequence)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		blockHash, txHash, index, prevHash[:], int64(txIn.PreviousOutPoint.Index), txIn.SignatureScript, int64(txIn.Sequence),
	)
	return err
}

func insertTxOut(ctx context.Context, ex sqlExecer, blockHash, txHash []byte, index int, txOut *wire.TxOut) error {
	_, err := ex.ExecContext(ctx, `
		INSERT INTO txouts (block_hash, tx_hash, tx_index, value, pk_script)
		VALUES ($1, $2, $3, $4, $5)`,
		blockHash, txHash, index, txOut.Value, txOut.PkScript,
	)
	return err
}
