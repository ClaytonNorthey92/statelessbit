package database

import (
	"fmt"
	"os"

	"testing"
)

func TestNewRandomDB(t *testing.T) {
	connectionURI, d, err := NewRandomDatabase(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer d()

	t.Logf("going to create database from connection URI: %s", connectionURI)

	path, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	err = NewDatabase(connectionURI, fmt.Sprintf("file://%s/migrations", path))
	if err != nil {
		t.Fatalf("could not create new database: %s", err)
	}
}
