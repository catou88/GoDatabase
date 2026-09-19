package sql

import (
	"fmt"
	"strconv"
)

// Statement is implemented by every parsed SQL statement.
type Statement interface{ statement() }

// CreateTableStatement describes a CREATE TABLE statement.
type CreateTableStatement struct {
	Name    string
	Columns []ColumnDefinition
}

func (CreateTableStatement) statement() {}

// ColumnDefinition describes one column in a table definition.
type ColumnDefinition struct {
	Name       string
	Type       TokenType
	PrimaryKey bool
	NotNull    bool
}

// InsertStatement describes an INSERT statement.
type InsertStatement struct {
	Table   string
	Columns []string
	Values  []Literal
}

func (InsertStatement) statement() {}

// SelectStatement describes a SELECT statement.
type SelectStatement struct {
	Table   string
	Columns []string
	All     bool
	Where   []Predicate
}

func (SelectStatement) statement() {}

// Predicate is a column-to-literal comparison in a WHERE clause.
type Predicate struct {
	Column string
	Op     TokenType
	Value  Literal
}

// Literal is a parsed SQL literal. Value contains int64, string, bool, or nil.
type Literal struct {
	Kind  TokenType
	Value any
}

// Parse tokenizes and parses one supported SQL statement.
func Parse(input string) (Statement, error) {
	tokens, err := Lex(input)
	if err != nil {
		return nil, err
	}
	p := parser{tokens: tokens}
	statement, err := p.statement()
	if err != nil {
		return nil, err
	}
	if err := p.expect(EOF); err != nil {
		return nil, err
	}
	return statement, nil
}

type parser struct {
	tokens []Token
	pos    int
}

func (p *parser) statement() (Statement, error) {
	switch p.current().Type {
	case Create:
		return p.createTable()
	case Insert:
		return p.insert()
	case Select:
		return p.selectStatement()
	default:
		return nil, p.errorf("expected CREATE, INSERT, or SELECT")
	}
}

func (p *parser) createTable() (Statement, error) {
	if err := p.expect(Create); err != nil {
		return nil, err
	}
	if err := p.expect(Table); err != nil {
		return nil, err
	}
	name, err := p.identifier("table name")
	if err != nil {
		return nil, err
	}
	if err := p.expect(LParen); err != nil {
		return nil, err
	}
	var columns []ColumnDefinition
	for {
		columnName, err := p.identifier("column name")
		if err != nil {
			return nil, err
		}
		columnType := p.current().Type
		if columnType != Int64 && columnType != StringType && columnType != Bytes && columnType != Bool {
			return nil, p.errorf("expected column type, got %s", p.current().Type)
		}
		p.advance()
		column := ColumnDefinition{Name: columnName, Type: columnType}
		for p.current().Type == Primary || p.current().Type == Not {
			switch p.current().Type {
			case Primary:
				if column.PrimaryKey {
					return nil, p.errorf("duplicate PRIMARY KEY constraint")
				}
				p.advance()
				if err := p.expect(Key); err != nil {
					return nil, err
				}
				column.PrimaryKey = true
			case Not:
				if column.NotNull {
					return nil, p.errorf("duplicate NOT NULL constraint")
				}
				p.advance()
				if err := p.expect(Null); err != nil {
					return nil, err
				}
				column.NotNull = true
			}
		}
		if column.PrimaryKey {
			column.NotNull = true
		}
		columns = append(columns, column)
		if p.current().Type != Comma {
			break
		}
		p.advance()
	}
	if err := p.expect(RParen); err != nil {
		return nil, err
	}
	p.optionalSemicolon()
	return CreateTableStatement{Name: name, Columns: columns}, nil
}

func (p *parser) insert() (Statement, error) {
	if err := p.expect(Insert); err != nil {
		return nil, err
	}
	if err := p.expect(Into); err != nil {
		return nil, err
	}
	table, err := p.identifier("table name")
	if err != nil {
		return nil, err
	}
	columns, err := p.identifierList("column list")
	if err != nil {
		return nil, err
	}
	if err := p.expect(Values); err != nil {
		return nil, err
	}
	if err := p.expect(LParen); err != nil {
		return nil, err
	}
	var values []Literal
	for {
		value, err := p.literal()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if p.current().Type != Comma {
			break
		}
		p.advance()
	}
	if err := p.expect(RParen); err != nil {
		return nil, err
	}
	p.optionalSemicolon()
	if len(columns) != len(values) {
		return nil, p.errorf("column count %d does not match value count %d", len(columns), len(values))
	}
	return InsertStatement{Table: table, Columns: columns, Values: values}, nil
}

func (p *parser) selectStatement() (Statement, error) {
	if err := p.expect(Select); err != nil {
		return nil, err
	}
	statement := SelectStatement{}
	var err error
	if p.current().Type == Star {
		statement.All = true
		p.advance()
	} else {
		columns, err := p.identifierSequence("select list")
		if err != nil {
			return nil, err
		}
		statement.Columns = columns
	}
	if err := p.expect(From); err != nil {
		return nil, err
	}
	statement.Table, err = p.identifier("table name")
	if err != nil {
		return nil, err
	}
	if p.current().Type == Where {
		p.advance()
		for {
			column, err := p.identifier("predicate column")
			if err != nil {
				return nil, err
			}
			op := p.current().Type
			if !isComparison(op) {
				return nil, p.errorf("expected comparison operator, got %s", p.current().Type)
			}
			p.advance()
			value, err := p.literal()
			if err != nil {
				return nil, err
			}
			statement.Where = append(statement.Where, Predicate{Column: column, Op: op, Value: value})
			if p.current().Type != And {
				break
			}
			p.advance()
		}
	}
	p.optionalSemicolon()
	return statement, nil
}

func (p *parser) identifierSequence(context string) ([]string, error) {
	var names []string
	for {
		name, err := p.identifier(context)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
		if p.current().Type != Comma {
			return names, nil
		}
		p.advance()
	}
}

func (p *parser) identifierList(context string) ([]string, error) {
	if err := p.expect(LParen); err != nil {
		return nil, fmt.Errorf("sql: expected %s: %w", context, err)
	}
	var names []string
	for {
		name, err := p.identifier(context)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
		if p.current().Type != Comma {
			break
		}
		p.advance()
	}
	if err := p.expect(RParen); err != nil {
		return nil, err
	}
	return names, nil
}

func (p *parser) literal() (Literal, error) {
	token := p.current()
	switch token.Type {
	case String:
		p.advance()
		return Literal{Kind: String, Value: token.Text}, nil
	case Integer:
		value, err := strconv.ParseInt(token.Text, 10, 64)
		if err != nil {
			return Literal{}, p.errorf("invalid integer %q", token.Text)
		}
		p.advance()
		return Literal{Kind: Integer, Value: value}, nil
	case True, False:
		p.advance()
		return Literal{Kind: token.Type, Value: token.Type == True}, nil
	case Null:
		p.advance()
		return Literal{Kind: Null, Value: nil}, nil
	default:
		return Literal{}, p.errorf("expected literal, got %s", token.Type)
	}
}

func (p *parser) identifier(context string) (string, error) {
	if p.current().Type != Identifier {
		return "", p.errorf("expected %s, got %s", context, p.current().Type)
	}
	text := p.current().Text
	p.advance()
	return text, nil
}

func (p *parser) expect(kind TokenType) error {
	if p.current().Type != kind {
		return p.errorf("expected %s, got %s", kind, p.current().Type)
	}
	p.advance()
	return nil
}

func (p *parser) optionalSemicolon() {
	if p.current().Type == Semicolon {
		p.advance()
	}
}

func (p *parser) current() Token { return p.tokens[p.pos] }
func (p *parser) advance()       { p.pos++ }

func (p *parser) errorf(format string, args ...any) error {
	token := p.current()
	return fmt.Errorf("sql: parse error at byte %d: %s", token.Pos, fmt.Sprintf(format, args...))
}

func isComparison(kind TokenType) bool {
	return kind == Equal || kind == NotEqual || kind == Less || kind == LessEqual || kind == Greater || kind == GreaterEqual
}
