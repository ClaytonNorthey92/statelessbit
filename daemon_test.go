package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"mydatabase"
)

func startBitcoindContainer(t *testing.T, ctx context.Context) *rpcclient.Client {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "kylemanna/bitcoind",
			Cmd: []string{
				"bitcoind",
				"-regtest",
				"-rpcallowip=0.0.0.0/0",
				"-rpcbind=0.0.0.0",
				"-rpcport=18443",
				"-rpcuser=bitcoin",
				"-rpcpassword=bitcoin",
				"-server=1",
				"-txindex=1",
			},
			ExposedPorts: []string{"18443/tcp"},
			WaitingFor:   wait.ForLog("Done loading"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("starting bitcoind container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminating bitcoind container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("getting container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "18443/tcp")
	if err != nil {
		t.Fatalf("getting mapped port: %v", err)
	}

	client, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", host, port.Port()),
		User:         "bitcoin",
		Pass:         "bitcoin",
		HTTPPostMode: true,
		DisableTLS:   true,
	}, nil)
	if err != nil {
		t.Fatalf("creating rpc client: %v", err)
	}
	t.Cleanup(client.Shutdown)

	// Bitcoin Core 26 requires an explicit wallet; with one wallet loaded,
	// wallet RPCs to the root endpoint auto-select it.
	walletName, _ := json.Marshal("test")
	if _, err := client.RawRequest("createwallet", []json.RawMessage{walletName}); err != nil {
		t.Fatalf("creating wallet: %v", err)
	}

	return client
}

func TestDaemonIndexesChain(t *testing.T) {
	const numBlocks = 5

	ctx := t.Context()

	client := startBitcoindContainer(t, ctx)

	db, drop, err := database.CreateNewRandomDatabase(ctx)
	if err != nil {
		t.Fatalf("creating database: %v", err)
	}
	defer drop()

	addr, err := client.GetNewAddress("")
	if err != nil {
		t.Fatalf("getting new address: %v", err)
	}

	blockHashes, err := client.GenerateToAddress(numBlocks, addr, nil)
	if err != nil {
		t.Fatalf("generating %d blocks: %v", numBlocks, err)
	}

	d := NewDaemon(db, client)
	if err := d.sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Verify total block count: genesis + numBlocks
	var totalBlocks int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM block_headers`).Scan(&totalBlocks); err != nil {
		t.Fatalf("counting block_headers: %v", err)
	}
	if totalBlocks != numBlocks+1 {
		t.Errorf("block_headers count = %d, want %d", totalBlocks, numBlocks+1)
	}

	// Verify each generated block and its transactions are fully indexed
	for _, hash := range blockHashes {
		block, err := client.GetBlock(hash)
		if err != nil {
			t.Fatalf("fetching block %s from rpc: %v", hash, err)
		}

		assertBlockHeader(ctx, t, db, block)

		for _, tx := range block.Transactions {
			assertTxOuts(ctx, t, db, hash[:], tx)
			assertTxIns(ctx, t, db, hash[:], tx)
		}
	}
}
