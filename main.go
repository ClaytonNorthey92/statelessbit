package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
	_ "github.com/lib/pq"
	"mydatabase"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dbURL := getEnv("DATABASE_URL", "postgres://user:password@localhost:5432/admin?sslmode=disable")

	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("getting working directory: %v", err)
	}

	if err := database.NewDatabase(dbURL, fmt.Sprintf("file://%s/database/migrations", wd)); err != nil {
		log.Fatalf("running migrations: %v", err)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("opening database connection: %v", err)
	}
	defer db.Close()

	client, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:         getEnv("BITCOIN_RPC_HOST", "localhost:18443"),
		User:         getEnv("BITCOIN_RPC_USER", "bitcoin"),
		Pass:         getEnv("BITCOIN_RPC_PASS", "bitcoin"),
		HTTPPostMode: true,
		DisableTLS:   true,
	}, nil)
	if err != nil {
		log.Fatalf("creating bitcoin rpc client: %v", err)
	}
	defer client.Shutdown()

	log.Printf("starting daemon (rpc=%s)", getEnv("BITCOIN_RPC_HOST", "localhost:18443"))

	if err := NewDaemon(db, client, networkChainParams()).Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("daemon exited: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func networkChainParams() *chaincfg.Params {
	switch os.Getenv("BITCOIN_NETWORK") {
	case "mainnet":
		return &chaincfg.MainNetParams
	case "testnet":
		return &chaincfg.TestNet3Params
	case "testnet4":
		return &chaincfg.TestNet4Params
	default:
		return &chaincfg.RegressionNetParams
	}
}
