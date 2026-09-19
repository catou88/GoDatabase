package sql

import (
	"bytes"
	"fmt"
	"strings"

	"godatabase/db"
)

// Result contains the rows produced by a SELECT statement. CREATE TABLE and
// INSERT return an empty result.
type Result struct {
	Rows []map[string]any
}

// Execute runs a parsed statement against database.
func Execute(database *db.Database, statement Statement) (Result, error) {
	if database == nil {
		return Result{}, fmt.Errorf("sql: nil database")
	}
	switch statement := statement.(type) {
	case nil:
		return Result{}, fmt.Errorf("sql: nil statement")
	case *CreateTableStatement:
		if statement == nil {
			return Result{}, fmt.Errorf("sql: nil CREATE TABLE statement")
		}
		return executeCreateTable(database, *statement)
	case *InsertStatement:
		if statement == nil {
			return Result{}, fmt.Errorf("sql: nil INSERT statement")
		}
		return executeInsert(database, *statement)
	case *SelectStatement:
		if statement == nil {
			return Result{}, fmt.Errorf("sql: nil SELECT statement")
		}
		return executeSelect(database, *statement)
	case CreateTableStatement:
		return executeCreateTable(database, statement)
	case InsertStatement:
		return executeInsert(database, statement)
	case SelectStatement:
		return executeSelect(database, statement)
	default:
		return Result{}, fmt.Errorf("sql: unsupported statement %T", statement)
	}
}

// Exec parses and executes one SQL statement.
func Exec(database *db.Database, input string) (Result, error) {
	statement, err := Parse(input)
	if err != nil {
		return Result{}, err
	}
	return Execute(database, statement)
}

func executeCreateTable(database *db.Database, statement CreateTableStatement) (Result, error) {
	columns := make([]db.Column, len(statement.Columns))
	for i, column := range statement.Columns {
		columnType, err := databaseColumnType(column.Type)
		if err != nil {
			return Result{}, err
		}
		columns[i] = db.Column{Name: column.Name, Type: columnType, PrimaryKey: column.PrimaryKey, Nullable: !column.NotNull && !column.PrimaryKey}
	}
	_, err := database.CreateTable(db.TableSchema{Name: statement.Name, Columns: columns})
	return Result{}, err
}

func executeInsert(database *db.Database, statement InsertStatement) (Result, error) {
	if statement.Table == "" {
		return Result{}, fmt.Errorf("sql: INSERT requires a table name")
	}
	if len(statement.Columns) == 0 || len(statement.Columns) != len(statement.Values) {
		return Result{}, fmt.Errorf("sql: INSERT requires a nonempty column list with matching value count")
	}
	table, err := database.OpenTable(statement.Table)
	if err != nil {
		return Result{}, err
	}
	schema := table.Schema()
	row := make(map[string]any, len(statement.Columns))
	for i, name := range statement.Columns {
		column := findColumn(schema, name)
		if column == nil {
			return Result{}, fmt.Errorf("sql: unknown column %q", name)
		}
		if _, exists := row[name]; exists {
			return Result{}, fmt.Errorf("sql: duplicate column %q", name)
		}
		value, err := literalValue(statement.Values[i], column.Type)
		if err != nil {
			return Result{}, fmt.Errorf("sql: column %q: %w", name, err)
		}
		row[name] = value
	}
	for _, column := range schema.Columns {
		if _, exists := row[column.Name]; !exists {
			if column.Nullable {
				return Result{}, fmt.Errorf("sql: omitted nullable column %q: NULL values are unsupported", column.Name)
			}
			return Result{}, fmt.Errorf("sql: missing required column %q", column.Name)
		}
	}
	if err := table.Insert(row); err != nil {
		return Result{}, err
	}
	return Result{}, nil
}

func executeSelect(database *db.Database, statement SelectStatement) (Result, error) {
	if statement.Table == "" {
		return Result{}, fmt.Errorf("sql: SELECT requires a table name")
	}
	if statement.All && len(statement.Columns) != 0 || !statement.All && len(statement.Columns) == 0 {
		return Result{}, fmt.Errorf("sql: SELECT requires either * or a nonempty column list")
	}
	table, err := database.OpenTable(statement.Table)
	if err != nil {
		return Result{}, err
	}
	schema := table.Schema()
	if !statement.All {
		for _, name := range statement.Columns {
			if findColumn(schema, name) == nil {
				return Result{}, fmt.Errorf("sql: unknown column %q", name)
			}
		}
	}
	primary := primaryColumn(schema)
	if primary == nil {
		return Result{}, fmt.Errorf("sql: table %q has no primary key", statement.Table)
	}
	// Validate every predicate before Get or Scan, even when no rows match.
	for _, predicate := range statement.Where {
		column := findColumn(schema, predicate.Column)
		if column == nil {
			return Result{}, fmt.Errorf("sql: unknown predicate column %q", predicate.Column)
		}
		if !isComparison(predicate.Op) {
			return Result{}, fmt.Errorf("sql: unsupported comparison operator %s (%d)", predicate.Op, predicate.Op)
		}
		if _, err := literalValue(predicate.Value, column.Type); err != nil {
			return Result{}, fmt.Errorf("sql: predicate column %q: %w", predicate.Column, err)
		}
	}
	rows, err := selectRows(table, primary, statement.Where)
	if err != nil {
		return Result{}, err
	}
	result := Result{Rows: make([]map[string]any, 0, len(rows))}
	for _, row := range rows {
		if !matches(row, statement.Where, schema) {
			continue
		}
		if !statement.All {
			row = project(row, statement.Columns)
		}
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}

func selectRows(table *db.Table, primary *db.Column, predicates []Predicate) ([]map[string]any, error) {
	for _, predicate := range predicates {
		if predicate.Column != primary.Name || predicate.Op != Equal {
			continue
		}
		key, err := literalValue(predicate.Value, primary.Type)
		if err != nil {
			return nil, fmt.Errorf("sql: primary key predicate: %w", err)
		}
		row, found, err := table.Get(key)
		if err != nil || !found {
			return nil, err
		}
		return []map[string]any{row}, nil
	}
	var lower, upper any
	for _, predicate := range predicates {
		if predicate.Column != primary.Name {
			continue
		}
		switch predicate.Op {
		case Greater, GreaterEqual, Less, LessEqual:
			value, err := literalValue(predicate.Value, primary.Type)
			if err != nil {
				return nil, fmt.Errorf("sql: primary key predicate: %w", err)
			}
			if predicate.Op == Greater || predicate.Op == GreaterEqual {
				if lower == nil || compare(value, lower, Greater) {
					lower = value
				}
			} else if upper == nil || compare(value, upper, Less) {
				upper = value
			}
		}
	}
	if lower != nil && upper != nil {
		// Range includes endpoints; matches applies strict comparisons afterward.
		return table.Range(lower, upper)
	}
	return table.Scan()
}

func matches(row map[string]any, predicates []Predicate, schema db.TableSchema) bool {
	for _, predicate := range predicates {
		column := findColumn(schema, predicate.Column)
		if column == nil {
			return false
		}
		value, err := literalValue(predicate.Value, column.Type)
		if err != nil || !compare(row[predicate.Column], value, predicate.Op) {
			return false
		}
	}
	return true
}

func compare(left, right any, operator TokenType) bool {
	var order int
	switch left := left.(type) {
	case int64:
		right, ok := right.(int64)
		if !ok {
			return false
		}
		if left < right {
			order = -1
		} else if left > right {
			order = 1
		}
	case string:
		right, ok := right.(string)
		if !ok {
			return false
		}
		order = strings.Compare(left, right)
	case []byte:
		right, ok := right.([]byte)
		if !ok {
			return false
		}
		order = bytes.Compare(left, right)
	case bool:
		right, ok := right.(bool)
		if !ok {
			return false
		}
		if left != right {
			order = -1
			if left {
				order = 1
			}
		}
	default:
		return false
	}
	switch operator {
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
	}
	return false
}

func project(row map[string]any, columns []string) map[string]any {
	projected := make(map[string]any, len(columns))
	for _, column := range columns {
		projected[column] = row[column]
	}
	return projected
}

func literalValue(literal Literal, columnType db.ColumnType) (any, error) {
	switch literal.Kind {
	case Null:
		return nil, fmt.Errorf("NULL literals are unsupported")
	case Integer:
		if _, ok := literal.Value.(int64); !ok {
			return nil, fmt.Errorf("integer literal requires int64 value, got %T", literal.Value)
		}
	case String:
		if _, ok := literal.Value.(string); !ok {
			return nil, fmt.Errorf("string literal requires string value, got %T", literal.Value)
		}
	case True, False:
		value, ok := literal.Value.(bool)
		if !ok || value != (literal.Kind == True) {
			return nil, fmt.Errorf("%s literal requires matching bool value, got %v (%T)", literal.Kind, literal.Value, literal.Value)
		}
	default:
		return nil, fmt.Errorf("unsupported literal kind %s (%d)", literal.Kind, literal.Kind)
	}
	switch columnType {
	case db.ColumnInt64:
		if literal.Kind == Integer {
			return literal.Value, nil
		}
	case db.ColumnString:
		if literal.Kind == String {
			return literal.Value, nil
		}
	case db.ColumnBytes:
		if literal.Kind == String {
			return []byte(literal.Value.(string)), nil
		}
	case db.ColumnBool:
		if literal.Kind == True || literal.Kind == False {
			return literal.Value, nil
		}
	}
	return nil, fmt.Errorf("literal %s is incompatible with column type %d", literal.Kind, columnType)
}

func databaseColumnType(kind TokenType) (db.ColumnType, error) {
	switch kind {
	case Int64:
		return db.ColumnInt64, nil
	case StringType:
		return db.ColumnString, nil
	case Bytes:
		return db.ColumnBytes, nil
	case Bool:
		return db.ColumnBool, nil
	}
	return 0, fmt.Errorf("sql: unsupported column type %s", kind)
}

func findColumn(schema db.TableSchema, name string) *db.Column {
	for i := range schema.Columns {
		if schema.Columns[i].Name == name {
			return &schema.Columns[i]
		}
	}
	return nil
}

func primaryColumn(schema db.TableSchema) *db.Column {
	for i := range schema.Columns {
		if schema.Columns[i].PrimaryKey {
			return &schema.Columns[i]
		}
	}
	return nil
}
