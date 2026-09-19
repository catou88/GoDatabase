package sql

import (
	"reflect"
	"strings"
	"testing"
)

func TestLexCreateTable(t *testing.T) {
	input := "  CrEaTe\n TABLE users (id INT64 PRIMARY KEY, name STRING NOT NULL);"
	got, err := Lex(input)
	if err != nil {
		t.Fatalf("Lex() error = %v", err)
	}
	want := []TokenType{Create, Table, Identifier, LParen, Identifier, Int64, Primary, Key, Comma, Identifier, StringType, Not, Null, RParen, Semicolon, EOF}
	if types := tokenTypes(got); !reflect.DeepEqual(types, want) {
		t.Fatalf("token types = %v, want %v", types, want)
	}
	if got[2].Text != "users" || got[2].Pos != 16 {
		t.Fatalf("table token = %#v, want users at byte 16", got[2])
	}
}

func TestLexValuesAndOperators(t *testing.T) {
	got, err := Lex("SELECT * FROM t WHERE id >= -42 AND name != 'Ada''s';")
	if err != nil {
		t.Fatalf("Lex() error = %v", err)
	}
	want := []struct {
		kind TokenType
		text string
	}{
		{Select, "SELECT"}, {Star, "*"}, {From, "FROM"}, {Identifier, "t"}, {Where, "WHERE"},
		{Identifier, "id"}, {GreaterEqual, ">="}, {Integer, "-42"}, {And, "AND"},
		{Identifier, "name"}, {NotEqual, "!="}, {String, "Ada's"}, {Semicolon, ";"}, {EOF, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Type != want[i].kind || got[i].Text != want[i].text {
			t.Errorf("token %d = (%s, %q), want (%s, %q)", i, got[i].Type, got[i].Text, want[i].kind, want[i].text)
		}
	}
}

func TestLexErrors(t *testing.T) {
	for _, input := range []string{"SELECT @", "'unterminated", "9223372036854775808", "!"} {
		t.Run(input, func(t *testing.T) {
			if _, err := Lex(input); err == nil {
				t.Fatalf("Lex(%q) returned nil error", input)
			}
		})
	}
}

func TestLexIdentifiersRemainCaseSensitive(t *testing.T) {
	got, err := Lex("Users users")
	if err != nil {
		t.Fatalf("Lex() error = %v", err)
	}
	if got[0].Type != Identifier || got[1].Type != Identifier || got[0].Text != "Users" || got[1].Text != "users" {
		t.Fatalf("identifiers = %#v", got[:2])
	}
}

func tokenTypes(tokens []Token) []TokenType {
	types := make([]TokenType, len(tokens))
	for i, token := range tokens {
		types[i] = token.Type
	}
	return types
}

func TestLexWhitespace(t *testing.T) {
	compact, err := Lex("SELECT a FROM t")
	if err != nil {
		t.Fatal(err)
	}
	spaced, err := Lex(strings.Join([]string{" SELECT", "a", " FROM", "t "}, "\t\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tokenTypes(compact), tokenTypes(spaced)) {
		t.Fatalf("whitespace changed token types: %v vs %v", tokenTypes(compact), tokenTypes(spaced))
	}
}
