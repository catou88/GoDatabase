package sql

import (
	"path/filepath"
	"testing"

	"godatabase/db"
)

func TestExecuteCreateInsertSelectAndRange(t *testing.T) {
	database := db.New()
	if _, err := Exec(database, "CREATE TABLE users (id INT64 PRIMARY KEY, name STRING, active BOOL);"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"INSERT INTO users (id, name, active) VALUES (2, 'Grace', FALSE);",
		"INSERT INTO users (id, name, active) VALUES (1, 'Ada', TRUE);",
	} {
		if _, err := Exec(database, query); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Exec(database, "SELECT name FROM users WHERE id >= 1 AND id <= 2;")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 || result.Rows[0]["name"] != "Ada" || result.Rows[1]["name"] != "Grace" {
		t.Fatalf("rows = %#v", result.Rows)
	}
	result, err = Exec(database, "SELECT * FROM users WHERE id = 2")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["name"] != "Grace" {
		t.Fatalf("point rows = %#v", result.Rows)
	}
}

func TestExecutePersistsAndReportsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Exec(database, "CREATE TABLE users (id INT64 PRIMARY KEY, name STRING);"); err != nil {
		t.Fatal(err)
	}
	if _, err := Exec(database, "INSERT INTO users (id, name) VALUES (7, 'Lin');"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	result, err := Exec(database, "SELECT * FROM users WHERE id = 7;")
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["name"] != "Lin" {
		t.Fatalf("reopened result = %#v, err = %v", result.Rows, err)
	}
	if _, err := Exec(database, "INSERT INTO users (id, name) VALUES ('wrong', 'value');"); err == nil {
		t.Fatal("expected type error")
	}
}
