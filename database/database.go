package database

import (
	"context"
	"fmt"
	"github.com/lib/pq"
	"math/rand/v2"
	"os"

	"database/sql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

type DropDBFunc func()

func getDatabaseURI(dbname string) string {
	return fmt.Sprintf("postgres://user:password@localhost:5432/%s?sslmode=disable", dbname)
}

func NewRandomDatabase(ctx context.Context) (string, DropDBFunc, error) {
	randomDBName := fmt.Sprintf("watcher-%d", rand.IntN(1000000))

	db, err := sql.Open("postgres", getDatabaseURI("admin"))
	if err != nil {
		return "", nil, fmt.Errorf("could not connect to database: %s", err)
	}

	quotedRandomDBName := pq.QuoteIdentifier(randomDBName)

	if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", quotedRandomDBName)); err != nil {
		return "", nil, fmt.Errorf("could not create database: %s", err)
	}

	return getDatabaseURI(randomDBName), func() {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", quotedRandomDBName)); err != nil {
			panic(fmt.Sprintf("could not drop database: %s", err))
		}
	}, nil
}

func CreateNewRandomDatabase(ctx context.Context) (*sql.DB, DropDBFunc, error) {
	connectionURI, d, err := NewRandomDatabase(ctx)
	if err != nil {
		return nil, nil, err
	}

	path, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}

	err = NewDatabase(connectionURI, fmt.Sprintf("file://%s/migrations", path))
	if err != nil {
		return nil, nil , err
	}

	db, err := sql.Open("postgres", connectionURI)
	if err != nil {
		return nil, nil, fmt.Errorf("could not open postgres connection: %s", err)
	}

	return db, d, nil
}

func NewDatabase(connectionURI string, migrationsDirectory string) error {
	db, err := sql.Open("postgres", connectionURI)
	if err != nil {
		return fmt.Errorf("could not open postgres connection: %s", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("could not call WithInstance: %s", err)
	}

	m, err := migrate.NewWithDatabaseInstance(migrationsDirectory,
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("could not create new migrations instance: %s", err)
	}

	if err := m.Up(); err != nil {
		return fmt.Errorf("could not migratee up: %s", err)
	}

	return nil
}
