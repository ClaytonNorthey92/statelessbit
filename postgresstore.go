package statelessbit

// type Store interface {
// 	InsertBlock(*wire.MsgBlock) error
// 	GetBlock(*chainhash.Hash) (*wire.MsgBlock, error)
// 	Clear() error
// 	ListUnspent(btcutil.Address, chainhash.Hash) ([]*wire.TxOut, error)
// }

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

type PostgresStore struct {
	db          *sql.DB
	chainParams *chaincfg.Params
}

func NewPostgresStore(db *sql.DB, chainParams *chaincfg.Params) *PostgresStore {
	return &PostgresStore{db: db, chainParams: chainParams}
}

func (p *PostgresStore) InsertBlock(block *wire.MsgBlock) error {
	blockHash := block.Header.BlockHash().String()

	dbtx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer func() {
		if err := dbtx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			panic(err)
		}
	}()

	for _, tx := range block.Transactions {
		txHash := tx.TxHash().String()
		prevHash := block.Header.PrevBlock.String()

		for i, txOut := range tx.TxOut {
			_, addrs, _, err := txscript.ExtractPkScriptAddrs(txOut.PkScript, p.chainParams)
			if err != nil {
				return fmt.Errorf("could not extract pk script addresses: %w", err)
			}

			addrStrings := []string{}
			for _, a := range addrs {
				addrStrings = append(addrStrings, a.String())
			}

			if txOut.PkScript == nil {
				txOut.PkScript = []byte{}
			}

			_, err = dbtx.Exec(
				`INSERT INTO txouts (outpoint_index, outpoint_txhash, created_at_block_hash, created_at_prev_block_hash, owner_address, tx_value, pk_script)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				i, txHash, blockHash, prevHash, addrStrings, txOut.Value, txOut.PkScript,
			)
			if err != nil {
				return fmt.Errorf("could not insert txout %d of tx %s: %w", i, txHash, err)
			}
		}

		for _, txin := range tx.TxIn {
			if blockchain.IsCoinBaseTx(tx) {
				continue
			}

			outpoint := txin.PreviousOutPoint

			_, err = dbtx.Exec(
				`INSERT INTO txout_spends (outpoint_index, outpoint_txhash, spent_at_block_hash)
				 VALUES ($1, $2, $3)`,
				outpoint.Index, outpoint.Hash.String(), block.Header.BlockHash().String())
			if err != nil {
				return fmt.Errorf("could not insert txout_spend: %w", err)
			}
		}
	}

	if err := dbtx.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}

	if err := p.setHeights(); err != nil {
		return err
	}

	return nil
}

func (p *PostgresStore) DeleteBlock(hash chainhash.Hash) error {
	dbtx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}

	defer func() {
		if err := dbtx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			panic(err)
		}
	}()

	_, err = dbtx.Exec("DELETE FROM txouts WHERE created_at_block_hash = $1", hash.String())
	if err != nil {
		return err
	}

	_, err = dbtx.Exec("DELETE FROM txout_spends WHERE spent_at_block_hash = $1", hash.String())
	if err != nil {
		return err
	}

	if err := dbtx.Commit(); err != nil {
		return err
	}

	return nil
}

func (p *PostgresStore) ListUnspent(addr btcutil.Address, hash chainhash.Hash) ([]*wire.TxOut, error) {
	fmt.Printf("checking unspent with address %s and hash %s\n", addr.String(), hash.String())
	rows, err := p.db.Query(
		`
		SELECT tx_value, pk_script FROM txouts txo WHERE $1 = ANY(owner_address)
		AND active_block IS TRUE
		AND (
			NOT EXISTS (
				SELECT * FROM txout_spends WHERE
				(
					SELECT created_at_block_height FROM txouts txo2
					WHERE txo2.created_at_block_hash = spent_at_block_hash
				) <= (
					SELECT created_at_block_height FROM txouts
					WHERE created_at_block_hash = $2
					LIMIT 1
				)
				AND txo.outpoint_index = txout_spends.outpoint_index
				AND txo.outpoint_txhash = txout_spends.outpoint_txhash
			)
		)
		`,
		addr.String(), hash.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("could not query unspent txouts: %w", err)
	}
	defer rows.Close()

	var result []*wire.TxOut
	for rows.Next() {
		var value int64
		var pkScript []byte
		if err := rows.Scan(&value, &pkScript); err != nil {
			return nil, fmt.Errorf("could not scan txout row: %w", err)
		}
		result = append(result, &wire.TxOut{
			Value:    value,
			PkScript: pkScript,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating txout rows: %w", err)
	}

	return result, nil
}

func (p *PostgresStore) SetActiveFromTip(hash chainhash.Hash) error {
	dbtx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer func() {
		if err := dbtx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			panic(err)
		}
	}()

	if _, err := dbtx.Exec(`UPDATE txouts SET active_block = FALSE`); err != nil {
		return err
	}

	for {
		if _, err := dbtx.Exec(`UPDATE txouts SET active_block = TRUE WHERE created_at_block_hash = $1`, hash.String()); err != nil {
			return err
		}

		var prevHashStr string
		if err := dbtx.QueryRow(
			`SELECT created_at_prev_block_hash FROM txouts WHERE created_at_block_hash = $1 LIMIT 1`,
			hash.String(),
		).Scan(&prevHashStr); err != nil {
			return fmt.Errorf("could not get prev block hash for %s: %w", hash, err)
		}

		if prevHashStr == (chainhash.Hash{}).String() {
			break
		}

		prevHash, err := chainhash.NewHashFromStr(prevHashStr)
		if err != nil {
			return fmt.Errorf("could not parse prev block hash %q: %w", prevHashStr, err)
		}
		hash = *prevHash
	}

	if err := dbtx.Commit(); err != nil {
		return err
	}

	return nil
}

func (p *PostgresStore) setHeights() error {
	height := 0
	prevHash := chainhash.Hash{}.String()

	for {
		result, err := p.db.Exec(
			`UPDATE txouts SET created_at_block_height = $1 WHERE created_at_prev_block_hash = $2`,
			height, prevHash,
		)
		if err != nil {
			return fmt.Errorf("could not set heights at level %d: %w", height, err)
		}

		n, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("could not get rows affected: %w", err)
		}
		if n == 0 {
			break
		}

		err = p.db.QueryRow(
			`SELECT created_at_block_hash FROM txouts WHERE created_at_prev_block_hash = $1 LIMIT 1`,
			prevHash,
		).Scan(&prevHash)
		if err != nil {
			return fmt.Errorf("could not get next block hash: %w", err)
		}

		height++
	}

	return nil
}

func (p *PostgresStore) Clear() error {
	_, err := p.db.Exec(
		`DELETE FROM txouts`,
	)
	if err != nil {
		return fmt.Errorf("could not delete txouts")
	}

	return nil
}
