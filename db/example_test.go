package db_test

import (
	"fmt"
	"os"
	"path/filepath"

	"godatabase/db"
)

func ExampleOpen() {
	directory, err := os.MkdirTemp("", "godatabase-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(directory)

	database, err := db.Open(filepath.Join(directory, "example.db"))
	if err != nil {
		panic(err)
	}
	defer database.Close()

	if err := database.Set("language", "Go"); err != nil {
		panic(err)
	}
	value, found, err := database.Get("language")
	if err != nil {
		panic(err)
	}
	fmt.Println(value, found)
	// Output: Go true
}
