// Package sql contains the implementation of the database's SQL subset.
package sql

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// TokenType identifies the kind of SQL token.
type TokenType uint8

const (
	Illegal TokenType = iota
	EOF
	Identifier
	String
	Integer
	True
	False
	Create
	Table
	Primary
	Key
	Not
	Null
	Insert
	Into
	Values
	Select
	From
	Where
	And
	Int64
	StringType
	Bytes
	Bool
	LParen
	RParen
	Comma
	Semicolon
	Star
	Equal
	NotEqual
	Less
	LessEqual
	Greater
	GreaterEqual
)

var tokenNames = map[TokenType]string{
	Illegal: "illegal", EOF: "EOF", Identifier: "identifier", String: "string",
	Integer: "integer", True: "TRUE", False: "FALSE", Create: "CREATE", Table: "TABLE",
	Primary: "PRIMARY", Key: "KEY", Not: "NOT", Null: "NULL", Insert: "INSERT", Into: "INTO",
	Values: "VALUES", Select: "SELECT", From: "FROM", Where: "WHERE", And: "AND", Int64: "INT64",
	StringType: "STRING", Bytes: "BYTES", Bool: "BOOL", LParen: "(", RParen: ")", Comma: ",",
	Semicolon: ";", Star: "*", Equal: "=", NotEqual: "!=", Less: "<", LessEqual: "<=",
	Greater: ">", GreaterEqual: ">=",
}

// String returns the conventional name of a token type.
func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return "unknown"
}

// Token is one lexical unit. Pos is the zero-based byte offset in the input.
// Text is the decoded string value for String tokens and the source spelling
// for all other tokens.
type Token struct {
	Type TokenType
	Text string
	Pos  int
}

// Lex tokenizes input according to the SQL subset documented in
// docs/design/sql-subset.md. The returned stream always ends with EOF.
func Lex(input string) ([]Token, error) {
	lexer := lexer{input: input}
	return lexer.lex()
}

type lexer struct {
	input string
	pos   int
}

func (l *lexer) lex() ([]Token, error) {
	var tokens []Token
	for {
		l.skipWhitespace()
		if l.pos == len(l.input) {
			return append(tokens, Token{Type: EOF, Pos: l.pos}), nil
		}

		start := l.pos
		ch := l.input[l.pos]
		switch {
		case isIdentifierStart(ch):
			tokens = append(tokens, l.identifier())
		case ch == '\'':
			token, err := l.stringLiteral()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
		case isDigit(ch) || ((ch == '+' || ch == '-') && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1])):
			token, err := l.integer()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
		default:
			token, ok := l.punctuation()
			if !ok {
				return nil, fmt.Errorf("sql: invalid character %q at byte %d", ch, start)
			}
			tokens = append(tokens, token)
		}
	}
}

func (l *lexer) identifier() Token {
	start := l.pos
	l.pos++
	for l.pos < len(l.input) && isIdentifierPart(l.input[l.pos]) {
		l.pos++
	}
	text := l.input[start:l.pos]
	keywords := map[string]TokenType{
		"CREATE": Create, "TABLE": Table, "PRIMARY": Primary, "KEY": Key,
		"NOT": Not, "NULL": Null, "INSERT": Insert, "INTO": Into,
		"VALUES": Values, "SELECT": Select, "FROM": From, "WHERE": Where,
		"AND": And, "INT64": Int64, "STRING": StringType, "BYTES": Bytes,
		"BOOL": Bool, "TRUE": True, "FALSE": False,
	}
	if kind, ok := keywords[strings.ToUpper(text)]; ok {
		return Token{Type: kind, Text: text, Pos: start}
	}
	return Token{Type: Identifier, Text: text, Pos: start}
}

func (l *lexer) stringLiteral() (Token, error) {
	start := l.pos
	l.pos++
	var value strings.Builder
	for l.pos < len(l.input) {
		if l.input[l.pos] != '\'' {
			value.WriteByte(l.input[l.pos])
			l.pos++
			continue
		}
		l.pos++
		if l.pos < len(l.input) && l.input[l.pos] == '\'' {
			value.WriteByte('\'')
			l.pos++
			continue
		}
		return Token{Type: String, Text: value.String(), Pos: start}, nil
	}
	return Token{}, fmt.Errorf("sql: unterminated string at byte %d", start)
}

func (l *lexer) integer() (Token, error) {
	start := l.pos
	if l.input[l.pos] == '+' || l.input[l.pos] == '-' {
		l.pos++
	}
	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}
	text := l.input[start:l.pos]
	if _, err := strconv.ParseInt(text, 10, 64); err != nil {
		return Token{}, fmt.Errorf("sql: invalid int64 %q at byte %d", text, start)
	}
	return Token{Type: Integer, Text: text, Pos: start}, nil
}

func (l *lexer) punctuation() (Token, bool) {
	start := l.pos
	text := l.input[l.pos : l.pos+1]
	kind := Illegal
	switch l.input[l.pos] {
	case '(':
		kind = LParen
	case ')':
		kind = RParen
	case ',':
		kind = Comma
	case ';':
		kind = Semicolon
	case '*':
		kind = Star
	case '=':
		kind = Equal
	case '<':
		kind = Less
	case '>':
		kind = Greater
	case '!':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			kind = NotEqual
			text = l.input[l.pos : l.pos+2]
		}
	}
	if kind == Illegal {
		return Token{}, false
	}
	l.pos += len(text)
	if (kind == Less || kind == Greater) && l.pos < len(l.input) && l.input[l.pos] == '=' {
		text += "="
		l.pos++
		if kind == Less {
			kind = LessEqual
		} else {
			kind = GreaterEqual
		}
	}
	return Token{Type: kind, Text: text, Pos: start}, true
}

func (l *lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		r := rune(l.input[l.pos])
		if !unicode.IsSpace(r) {
			return
		}
		l.pos++
	}
}

func isIdentifierStart(ch byte) bool {
	return ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}
func isIdentifierPart(ch byte) bool { return isIdentifierStart(ch) || isDigit(ch) }
func isDigit(ch byte) bool          { return ch >= '0' && ch <= '9' }
