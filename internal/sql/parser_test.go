package sql

import (
	"reflect"
	"testing"
)

func TestParseCreateTable(t *testing.T) {
	statement, err := Parse("CREATE TABLE users (id INT64 PRIMARY KEY, name STRING NOT NULL, active BOOL);")
	if err != nil {
		t.Fatal(err)
	}
	want := CreateTableStatement{
		Name: "users",
		Columns: []ColumnDefinition{
			{Name: "id", Type: Int64, PrimaryKey: true, NotNull: true},
			{Name: "name", Type: StringType, NotNull: true},
			{Name: "active", Type: Bool},
		},
	}
	if got := statement.(CreateTableStatement); !reflect.DeepEqual(got, want) {
		t.Fatalf("statement = %#v, want %#v", got, want)
	}
}

func TestParseInsert(t *testing.T) {
	statement, err := Parse("insert into users (id, name, active) values (-42, 'Ada''s', TRUE)")
	if err != nil {
		t.Fatal(err)
	}
	want := InsertStatement{
		Table: "users", Columns: []string{"id", "name", "active"},
		Values: []Literal{{Kind: Integer, Value: int64(-42)}, {Kind: String, Value: "Ada's"}, {Kind: True, Value: true}},
	}
	if got := statement.(InsertStatement); !reflect.DeepEqual(got, want) {
		t.Fatalf("statement = %#v, want %#v", got, want)
	}
}

func TestParseSelectWithWhere(t *testing.T) {
	statement, err := Parse("SELECT id, name FROM users WHERE id >= 10 AND name = 'Ada';")
	if err != nil {
		t.Fatal(err)
	}
	want := SelectStatement{
		Table: "users", Columns: []string{"id", "name"},
		Where: []Predicate{
			{Column: "id", Op: GreaterEqual, Value: Literal{Kind: Integer, Value: int64(10)}},
			{Column: "name", Op: Equal, Value: Literal{Kind: String, Value: "Ada"}},
		},
	}
	if got := statement.(SelectStatement); !reflect.DeepEqual(got, want) {
		t.Fatalf("statement = %#v, want %#v", got, want)
	}
}

func TestParseSelectAll(t *testing.T) {
	statement, err := Parse("SELECT * FROM users")
	if err != nil {
		t.Fatal(err)
	}
	got := statement.(SelectStatement)
	if !got.All || got.Table != "users" || len(got.Where) != 0 {
		t.Fatalf("statement = %#v", got)
	}
}

func TestParseErrors(t *testing.T) {
	for _, input := range []string{
		"CREATE TABLE users (id INT64 PRIMARY);",
		"CREATE TABLE users (id INT64,);",
		"INSERT INTO users (id) VALUES ();",
		"INSERT INTO users (id) VALUES (1, 'extra');",
		"SELECT FROM users",
		"SELECT * users",
		"SELECT * FROM users WHERE id",
		"SELECT * FROM users WHERE id =",
		"DROP TABLE users",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := Parse(input); err == nil {
				t.Fatalf("Parse(%q) returned nil error", input)
			}
		})
	}
}
