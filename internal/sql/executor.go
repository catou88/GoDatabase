package sql

import (
	"fmt"

	"godatabase/db"
)

// Result contains the rows produced by a SELECT statement. CREATE TABLE and
// INSERT return an empty result.
type Result struct {
	Rows []map[string]any
}

// Execute runs a parsed statement against database.
func Execute(database *db.Database, statement Statement) (Result, error) {
	switch statement := statement.(type) {
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
	if err := table.Insert(row); err != nil {
		return Result{}, err
	}
	return Result{}, nil
}

func executeSelect(database *db.Database, statement SelectStatement) (Result, error) {
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
	start, end, err := rangeBounds(primary, predicates)
	if err != nil {
		return nil, err
	}
	return table.Range(start, end)
}

func rangeBounds(primary *db.Column, predicates []Predicate) (any, any, error) {
	start, end, err := minMax(primary.Type)
	if err != nil {
		return nil, nil, err
	}
	for _, predicate := range predicates {
		if predicate.Column != primary.Name {
			continue
		}
		value, valueErr := literalValue(predicate.Value, primary.Type)
		if valueErr != nil {
			return nil, nil, valueErr
		}
		switch predicate.Op {
		case Equal:
			start, end = value, value
		case Greater, GreaterEqual:
			start = value
		case Less, LessEqual:
			end = value
		default:
			return nil, nil, fmt.Errorf("sql: unsupported primary key operator %s", predicate.Op)
		}
	}
	return start, end, nil
}

func matches(row map[string]any, predicates []Predicate, schema db.TableSchema) bool {
	for _, predicate := range predicates {
		column := findColumn(schema, predicate.Column)
		value, err := literalValue(predicate.Value, column.Type)
		if err != nil || !compare(row[predicate.Column], value, predicate.Op) {
			return false
		}
	}
	return true
}

func compare(left, right any, operator TokenType) bool {
	switch left := left.(type) {
	case int64:
		right, ok := right.(int64)
		if !ok {
			return false
		}
		switch operator {
		case Equal:
			return left == right
		case NotEqual:
			return left != right
		case Less:
			return left < right
		case LessEqual:
			return left <= right
		case Greater:
			return left > right
		case GreaterEqual:
			return left >= right
		}
	case string:
		right, ok := right.(string)
		if !ok {
			return false
		}
		switch operator {
		case Equal:
			return left == right
		case NotEqual:
			return left != right
		case Less:
			return left < right
		case LessEqual:
			return left <= right
		case Greater:
			return left > right
		case GreaterEqual:
			return left >= right
		}
	case bool:
		right, ok := right.(bool)
		return ok && ((operator == Equal && left == right) || (operator == NotEqual && left != right))
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
	if literal.Kind == Null {
		return nil, nil
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

func minMax(kind db.ColumnType) (any, any, error) {
	switch kind {
	case db.ColumnInt64:
		return int64(-1 << 63), int64(1<<63 - 1), nil
	case db.ColumnString:
		return "", string([]byte{0xff, 0xff, 0xff, 0xff}), nil
	}
	return nil, nil, fmt.Errorf("sql: unsupported primary key type %d", kind)
}
