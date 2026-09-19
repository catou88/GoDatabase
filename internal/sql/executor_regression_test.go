package sql

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"godatabase/db"
)

func regressionDatabase(t *testing.T, populated bool) (*db.Database, *db.Table, []map[string]any) {
	t.Helper()
	database := db.New()
	t.Cleanup(func() { _ = database.Close() })
	table, err := database.CreateTable(db.TableSchema{Name: "items", Columns: []db.Column{
		{Name: "id", Type: db.ColumnInt64, PrimaryKey: true},
		{Name: "n", Type: db.ColumnInt64, Nullable: true},
		{Name: "s", Type: db.ColumnString, Nullable: true},
		{Name: "b", Type: db.ColumnBytes, Nullable: true},
		{Name: "flag", Type: db.ColumnBool, Nullable: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{
		{"id": int64(-2), "n": int64(-10), "s": "", "b": []byte{0}, "flag": false},
		{"id": int64(-1), "n": int64(0), "s": "a", "b": []byte("a"), "flag": true},
		{"id": int64(0), "n": int64(4), "s": "a\x00", "b": []byte("a\x00"), "flag": false},
		{"id": int64(1), "n": int64(4), "s": "aa", "b": []byte("aa"), "flag": true},
		{"id": int64(2), "n": int64(10), "s": "z", "b": []byte{0xff}, "flag": false},
	}
	if !populated {
		return database, table, nil
	}
	// Insert out of order so ordered results cannot merely follow insertion order.
	for _, i := range []int{3, 0, 4, 1, 2} {
		if err := table.Insert(rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	return database, table, rows
}

func regressionExecute(t *testing.T, database *db.Database, statement Statement) (result Result, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("Execute(%#v) panicked: %v", statement, recovered)
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()
	return Execute(database, statement)
}

func regressionRows(t *testing.T, got, want []map[string]any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %#v, want %#v", got, want)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("row %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func regressionInsert() InsertStatement {
	return InsertStatement{Table: "items", Columns: []string{"id", "n", "s", "b", "flag"}, Values: []Literal{
		{Kind: Integer, Value: int64(50)}, {Kind: Integer, Value: int64(7)},
		{Kind: String, Value: "new"}, {Kind: String, Value: "bytes"}, {Kind: True, Value: true},
	}}
}

func TestExecutorRegressionMalformedAST(t *testing.T) {
	cases := []struct {
		name string
		stmt Statement
	}{
		{"nil", nil},
		{"nil create pointer", (*CreateTableStatement)(nil)},
		{"nil insert pointer", (*InsertStatement)(nil)},
		{"nil select pointer", (*SelectStatement)(nil)},
		{"create missing name", CreateTableStatement{Columns: []ColumnDefinition{{Name: "id", Type: Int64, PrimaryKey: true}}}},
		{"create missing columns", CreateTableStatement{Name: "bad"}},
		{"create missing column name", CreateTableStatement{Name: "bad", Columns: []ColumnDefinition{{Type: Int64, PrimaryKey: true}}}},
		{"create invalid type", CreateTableStatement{Name: "bad", Columns: []ColumnDefinition{{Name: "id", Type: String, PrimaryKey: true}}}},
		{"select missing table", SelectStatement{All: true}},
		{"select missing projection", SelectStatement{Table: "items"}},
		{"select conflicting projection", SelectStatement{Table: "items", All: true, Columns: []string{"id"}}},
		{"select empty column", SelectStatement{Table: "items", Columns: []string{""}}},
		{"select unknown column", SelectStatement{Table: "items", Columns: []string{"absent"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database, _, _ := regressionDatabase(t, false)
			if _, err := regressionExecute(t, database, tc.stmt); err == nil {
				t.Fatal("expected malformed statement error")
			}
		})
	}
}

func TestExecutorRegressionPointerStatements(t *testing.T) {
	// Value statements are the public AST convention. Pointer forms may be
	// rejected, but must never panic or report success without doing the work.
	t.Run("create", func(t *testing.T) {
		database, _, _ := regressionDatabase(t, false)
		stmt := &CreateTableStatement{Name: "ptr", Columns: []ColumnDefinition{{Name: "id", Type: Int64, PrimaryKey: true}}}
		if _, err := regressionExecute(t, database, stmt); err == nil {
			if _, err := database.OpenTable("ptr"); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("insert", func(t *testing.T) {
		database, table, _ := regressionDatabase(t, false)
		stmt := regressionInsert()
		if _, err := regressionExecute(t, database, &stmt); err == nil {
			if _, found, err := table.Get(int64(50)); err != nil || !found {
				t.Fatalf("pointer insert: found=%v err=%v", found, err)
			}
		}
	})
	t.Run("select", func(t *testing.T) {
		database, _, rows := regressionDatabase(t, true)
		if result, err := regressionExecute(t, database, &SelectStatement{Table: "items", All: true}); err == nil {
			regressionRows(t, result.Rows, rows)
		}
	})
}

func TestExecutorRegressionBadInsertHasNoSideEffects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*InsertStatement)
	}{
		{"missing table", func(s *InsertStatement) { s.Table = "" }},
		{"missing columns", func(s *InsertStatement) { s.Columns = nil }},
		{"missing values", func(s *InsertStatement) { s.Values = nil }},
		{"empty insert", func(s *InsertStatement) { s.Columns, s.Values = nil, nil }},
		{"too few values", func(s *InsertStatement) { s.Values = s.Values[:4] }},
		{"too many values", func(s *InsertStatement) { s.Values = append(s.Values, Literal{Kind: Integer, Value: int64(9)}) }},
		{"empty column", func(s *InsertStatement) { s.Columns[2] = "" }},
		{"unknown column", func(s *InsertStatement) { s.Columns[2] = "absent" }},
		{"duplicate column", func(s *InsertStatement) { s.Columns[2] = "n" }},
		{"duplicate primary key", func(s *InsertStatement) { s.Values[0].Value = int64(0) }},
		{"int Go type mismatch", func(s *InsertStatement) { s.Values[1].Value = "7" }},
		{"int not int64", func(s *InsertStatement) { s.Values[1].Value = int(7) }},
		{"string Go type mismatch", func(s *InsertStatement) { s.Values[2].Value = int64(7) }},
		{"bytes Go type mismatch", func(s *InsertStatement) { s.Values[3].Value = []byte("bytes") }},
		{"bool Go type mismatch", func(s *InsertStatement) { s.Values[4].Value = "true" }},
		{"wrong literal kind", func(s *InsertStatement) { s.Values[1] = Literal{Kind: String, Value: "7"} }},
		{"missing literal", func(s *InsertStatement) { s.Values[2] = Literal{} }},
		{"nil literal value", func(s *InsertStatement) { s.Values[2].Value = nil }},
		{"NULL with payload", func(s *InsertStatement) { s.Values[2] = Literal{Kind: Null, Value: "ignored"} }},
	}
	for i, column := range regressionInsert().Columns {
		i := i
		cases = append(cases, struct {
			name string
			edit func(*InsertStatement)
		}{"NULL " + column, func(s *InsertStatement) { s.Values[i] = Literal{Kind: Null} }})
		cases = append(cases, struct {
			name string
			edit func(*InsertStatement)
		}{"omitted " + column, func(s *InsertStatement) {
			s.Columns = append(s.Columns[:i], s.Columns[i+1:]...)
			s.Values = append(s.Values[:i], s.Values[i+1:]...)
		}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database, table, rows := regressionDatabase(t, true)
			stmt := regressionInsert()
			tc.edit(&stmt)
			_, err := regressionExecute(t, database, stmt)
			if err == nil {
				t.Error("expected invalid INSERT error")
			} else if (strings.HasPrefix(tc.name, "NULL ") || strings.HasPrefix(tc.name, "omitted ")) && !strings.Contains(strings.ToLower(err.Error()), "null") && !strings.Contains(strings.ToLower(err.Error()), "missing") && !strings.Contains(strings.ToLower(err.Error()), "omitted") {
				t.Errorf("error lacks NULL/missing-value context: %v", err)
			}
			got, err := table.Range(int64(-1<<63), int64(1<<63-1))
			if err != nil {
				t.Fatal(err)
			}
			regressionRows(t, got, rows)
		})
	}
}

var regressionOperators = []TokenType{Equal, NotEqual, Less, LessEqual, Greater, GreaterEqual}

func TestExecutorRegressionPredicateValidation(t *testing.T) {
	bad := []Predicate{
		{Column: "absent", Op: Equal, Value: Literal{Kind: Integer, Value: int64(1)}},
		{Column: "", Op: Equal, Value: Literal{Kind: Integer, Value: int64(1)}},
	}
	for _, column := range []string{"id", "n", "s", "b", "flag"} {
		kind, value := Integer, any(int64(1))
		if column == "s" || column == "b" {
			kind, value = String, "a"
		}
		if column == "flag" {
			kind, value = True, true
		}
		bad = append(bad,
			Predicate{Column: column, Op: And, Value: Literal{Kind: kind, Value: value}},
			Predicate{Column: column, Op: TokenType(255), Value: Literal{Kind: kind, Value: value}},
			Predicate{Column: column, Op: Equal, Value: Literal{Kind: kind, Value: struct{}{}}},
			Predicate{Column: column, Op: Equal, Value: Literal{}},
		)
		wrong := Literal{Kind: String, Value: "wrong"}
		if kind == String {
			wrong = Literal{Kind: Integer, Value: int64(1)}
		}
		bad = append(bad, Predicate{Column: column, Op: Equal, Value: wrong})
		// Ordinary comparisons with NULL are explicitly invalid, including !=.
		for _, op := range regressionOperators {
			bad = append(bad, Predicate{Column: column, Op: op, Value: Literal{Kind: Null}})
		}
	}
	for _, populated := range []bool{false, true} {
		for i, predicate := range bad {
			for _, path := range []string{"scan", "point hit", "point miss", "empty range"} {
				t.Run(fmt.Sprintf("populated=%v/%d/%s", populated, i, path), func(t *testing.T) {
					database, _, _ := regressionDatabase(t, populated)
					var prefix []Predicate
					switch path {
					case "point hit":
						prefix = []Predicate{{Column: "id", Op: Equal, Value: Literal{Kind: Integer, Value: int64(0)}}}
					case "point miss":
						prefix = []Predicate{{Column: "id", Op: Equal, Value: Literal{Kind: Integer, Value: int64(100)}}}
					case "empty range":
						prefix = []Predicate{{Column: "id", Op: Greater, Value: Literal{Kind: Integer, Value: int64(100)}}}
					}
					stmt := SelectStatement{Table: "items", All: true, Where: append(prefix, predicate)}
					if _, err := regressionExecute(t, database, stmt); err == nil {
						t.Fatalf("expected predicate validation error for %#v", predicate)
					}
				})
			}
		}
	}
}

// The reference uses standard Go ordering, lexicographic bytes, and false < true.
// It deliberately does not call the executor's comparison or bound helpers.
func regressionReference(row map[string]any, p Predicate) bool {
	order := 0
	switch left := row[p.Column].(type) {
	case int64:
		right := p.Value.Value.(int64)
		if left < right {
			order = -1
		} else if left > right {
			order = 1
		}
	case string:
		order = strings.Compare(left, p.Value.Value.(string))
	case []byte:
		order = bytes.Compare(left, []byte(p.Value.Value.(string)))
	case bool:
		right := p.Value.Value.(bool)
		if left != right {
			if left {
				order = 1
			} else {
				order = -1
			}
		}
	default:
		panic("unexpected reference type")
	}
	switch p.Op {
	case Equal:
		return order == 0
	case NotEqual:
		return order != 0
	case Less:
		return order < 0
	case LessEqual:
		return order <= 0
	case Greater:
		return order > 0
	case GreaterEqual:
		return order >= 0
	default:
		panic("unexpected reference operator")
	}
}

func regressionCheckFilter(t *testing.T, database *db.Database, rows []map[string]any, table string, predicates []Predicate) {
	t.Helper()
	var want []map[string]any
	for _, row := range rows {
		match := true
		for _, p := range predicates {
			match = regressionReference(row, p) && match
		}
		if match {
			want = append(want, row)
		}
	}
	result, err := regressionExecute(t, database, SelectStatement{Table: table, All: true, Where: predicates})
	if err != nil {
		t.Fatal(err)
	}
	regressionRows(t, result.Rows, want)
}

func TestExecutorRegressionComparisonReference(t *testing.T) {
	database, _, rows := regressionDatabase(t, true)
	for _, column := range []string{"id", "n", "s", "b", "flag"} {
		for i, row := range rows {
			value := row[column]
			kind := Integer
			switch v := value.(type) {
			case string:
				kind = String
			case []byte:
				kind, value = String, string(v)
			case bool:
				kind = False
				if v {
					kind = True
				}
			}
			for _, op := range regressionOperators {
				t.Run(fmt.Sprintf("%s/%d/%s", column, i, op), func(t *testing.T) {
					regressionCheckFilter(t, database, rows, "items", []Predicate{{Column: column, Op: op, Value: Literal{Kind: kind, Value: value}}})
				})
			}
		}
	}
}

func TestExecutorRegressionBoundsOrderIndependent(t *testing.T) {
	database, _, rows := regressionDatabase(t, true)
	p := func(op TokenType, value int64) Predicate {
		return Predicate{Column: "id", Op: op, Value: Literal{Kind: Integer, Value: value}}
	}
	cases := [][]Predicate{
		{p(GreaterEqual, -2), p(Greater, -1), p(LessEqual, 2), p(Less, 2)},
		{p(GreaterEqual, 1), p(LessEqual, -1), p(NotEqual, 0)},
		{p(Greater, 0), p(LessEqual, 0)},
		{p(GreaterEqual, 0), p(LessEqual, 0)},
		{p(Equal, 0), p(Equal, 1)},
		{p(Equal, 0), p(NotEqual, 0)},
		{p(Equal, 1), p(Greater, -2), p(Less, 2)},
		{p(Equal, 1), p(Less, 1)},
		{p(GreaterEqual, -1<<63), p(LessEqual, 1<<63-1)},
		{p(Greater, 1<<63-1)}, {p(Less, -1<<63)},
		{p(NotEqual, 0), {Column: "s", Op: GreaterEqual, Value: Literal{Kind: String, Value: "a"}}},
	}
	for i, predicates := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			var permute func(int)
			permute = func(at int) {
				if at == len(predicates) {
					regressionCheckFilter(t, database, rows, "items", predicates)
					return
				}
				for j := at; j < len(predicates); j++ {
					predicates[at], predicates[j] = predicates[j], predicates[at]
					permute(at + 1)
					predicates[at], predicates[j] = predicates[j], predicates[at]
				}
			}
			permute(0)
		})
	}
}

func TestExecutorRegressionStringPrimaryKeyScan(t *testing.T) {
	database := db.New()
	t.Cleanup(func() { _ = database.Close() })
	table, err := database.CreateTable(db.TableSchema{Name: "strings", Columns: []db.Column{{Name: "key", Type: db.ColumnString, PrimaryKey: true}}})
	if err != nil {
		t.Fatal(err)
	}
	// Raw string keys, including embedded NUL and values above the old four-FF
	// sentinel, must remain readable through both SQL lookup and full scans.
	keys := []string{"", "\x00", "a", "a\x00", "a\x00\xff", "aa", "\xff\xff\xff\xff", "\xff\xff\xff\xff\x00", "\xff\xff\xff\xff\xff"}
	rows := make([]map[string]any, len(keys))
	for i := len(keys) - 1; i >= 0; i-- {
		rows[i] = map[string]any{"key": keys[i]}
		if err := table.Insert(rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	regressionCheckFilter(t, database, rows, "strings", nil)
	for i, key := range keys {
		for _, op := range regressionOperators {
			t.Run(fmt.Sprintf("%d/%s", i, op), func(t *testing.T) {
				regressionCheckFilter(t, database, rows, "strings", []Predicate{{Column: "key", Op: op, Value: Literal{Kind: String, Value: key}}})
			})
		}
	}
	p := func(op TokenType, value string) Predicate {
		return Predicate{Column: "key", Op: op, Value: Literal{Kind: String, Value: value}}
	}
	for i, predicates := range [][]Predicate{
		{p(GreaterEqual, "a"), p(Greater, "a\x00"), p(LessEqual, "\xff\xff\xff\xff\xff")},
		{p(GreaterEqual, "\xff\xff\xff\xff"), p(LessEqual, "\xff\xff\xff\xff\xff")},
		{p(Greater, "a"), p(LessEqual, "a")},
		{p(GreaterEqual, "z"), p(LessEqual, "a")},
	} {
		t.Run(fmt.Sprintf("bounds/%d", i), func(t *testing.T) {
			regressionCheckFilter(t, database, rows, "strings", predicates)
			for left, right := 0, len(predicates)-1; left < right; left, right = left+1, right-1 {
				predicates[left], predicates[right] = predicates[right], predicates[left]
			}
			regressionCheckFilter(t, database, rows, "strings", predicates)
		})
	}
}

func TestExecutorRegressionParsedFlowAndProjection(t *testing.T) {
	database := db.New()
	t.Cleanup(func() { _ = database.Close() })
	for _, query := range []string{
		"CREATE TABLE people (id INT64 PRIMARY KEY, name STRING NOT NULL, payload BYTES, active BOOL);",
		"INSERT INTO people (payload, active, name, id) VALUES ('ab', TRUE, 'Ada''s', 2);",
		"INSERT INTO people (id, name, payload, active) VALUES (-1, 'Grace', 'aa', FALSE);",
		"INSERT INTO people (id, name, payload, active) VALUES (3, 'Lin', 'ac', TRUE);",
	} {
		if _, err := Exec(database, query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	cases := []struct {
		query string
		want  []map[string]any
	}{
		{"SELECT name, payload FROM people WHERE active = TRUE AND id < 3;", []map[string]any{{"name": "Ada's", "payload": []byte("ab")}}},
		{"SELECT name FROM people WHERE id != 2 AND payload >= 'aa';", []map[string]any{{"name": "Grace"}, {"name": "Lin"}}},
		{"SELECT * FROM people WHERE id = 2;", []map[string]any{{"id": int64(2), "name": "Ada's", "payload": []byte("ab"), "active": true}}},
		{"SELECT name FROM people WHERE id > 3 AND id <= -1;", nil},
		{"SELECT name FROM people WHERE id = 99;", nil},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			result, err := Exec(database, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			regressionRows(t, result.Rows, tc.want)
		})
	}
	for _, query := range []string{
		"SELECT absent FROM people WHERE id = 99;",
		"SELECT name FROM people WHERE id = 99 AND absent = 1;",
		"SELECT name FROM people WHERE active = 'TRUE';",
		"SELECT name FROM people WHERE name != NULL;",
		"INSERT INTO people (id, name, payload, active) VALUES (9, 'bad', NULL, TRUE);",
		"INSERT INTO people (id, name, active) VALUES (9, 'bad', TRUE);",
		"INSERT INTO people (id, name) VALUES (9);",
	} {
		t.Run(query, func(t *testing.T) {
			if _, err := Exec(database, query); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	result, err := Exec(database, "SELECT id FROM people;")
	if err != nil {
		t.Fatal(err)
	}
	regressionRows(t, result.Rows, []map[string]any{{"id": int64(-1)}, {"id": int64(2)}, {"id": int64(3)}})
}
