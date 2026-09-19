# SQL Subset Design

## Goal

Define the small SQL-like language supported by the first query layer.

The language is intentionally limited. Each supported construct should map
directly to the public table, primary-key, and secondary-index operations
without requiring a cost-based optimizer or a large expression engine.

## Supported Statements

The initial language supports:

- `CREATE TABLE`
- `INSERT INTO`
- `SELECT ... FROM ...`

Transaction control remains a Go API concern for the initial SQL layer. SQL
`BEGIN`, `COMMIT`, and `ROLLBACK` are deferred until the query execution model
is stable.

## Lexical Rules

- Keywords are case-insensitive: `select`, `SELECT`, and `Select` are equal.
- Table and column names are case-sensitive and use unquoted identifiers.
- Identifiers start with a letter or underscore and continue with letters,
  digits, or underscores.
- String literals use single quotes.
- A single quote inside a string is escaped by doubling it: `'Ada''s'`.
- Integer literals are signed base-10 `int64` values.
- Boolean literals are `TRUE` and `FALSE`.
- Whitespace separates tokens and is otherwise ignored.
- SQL comments are not supported initially.

## CREATE TABLE

### Syntax

```sql
CREATE TABLE table_name (
    column_name column_type [PRIMARY KEY],
    column_name column_type [NOT NULL]
);
```

Supported column types:

```text
INT64
STRING
BYTES
BOOL
```

Exactly one column must be declared `PRIMARY KEY`. The initial primary key
must be `INT64` or `STRING`. `PRIMARY KEY` implies `NOT NULL`.

Example:

```sql
CREATE TABLE users (
    id INT64 PRIMARY KEY,
    name STRING NOT NULL,
    active BOOL
);
```

The first SQL implementation does not support table options, defaults,
composite primary keys, generated columns, foreign keys, or inline secondary
index declarations. Secondary indexes are created through the Go API until
their SQL syntax is explicitly designed.

## INSERT

### Syntax

```sql
INSERT INTO table_name (column_name, column_name)
VALUES (literal, literal);
```

The number of columns and values must match. Columns may be listed in any
order, but each column may appear at most once. Every non-nullable column must
be supplied. A nullable column that is omitted receives `NULL` when nullable
row values are supported by the execution layer.

Currently, every column must be supplied, including columns declared nullable.
Execution rejects explicit `NULL` literals and omitted nullable columns because
the current row encoding cannot reliably preserve NULL values. This restriction
prevents SQL from writing unreadable or lossy values until nullable encoding is
implemented. String literals supply STRING columns directly and BYTES columns
as their literal bytes; no implicit numeric or boolean conversion is performed.

Example:

```sql
INSERT INTO users (id, name, active)
VALUES (42, 'Ada', TRUE);
```

`INSERT` rejects an existing primary key. An explicit future `UPSERT` or
`UPDATE` statement may provide replacement semantics; `INSERT` itself does not
overwrite rows.

## SELECT

### Syntax

```sql
SELECT select_list
FROM table_name
[WHERE predicate];
```

The select list is either `*` or one or more column names:

```sql
SELECT * FROM users;
SELECT id, name FROM users;
```

Results are returned in primary-key order unless a future statement explicitly
defines another ordering. `SELECT *` performs a table range scan.
Unbounded scans use `Table.Scan()`, which scans the table's encoded row prefix;
there is no fabricated maximum string key. The scan currently materializes its
result rather than providing a streaming iterator.

## WHERE

The initial `WHERE` clause supports one comparison or comparisons joined by
`AND`:

```sql
column = literal
column != literal
column < literal
column <= literal
column > literal
column >= literal
```

Examples:

```sql
SELECT * FROM users WHERE id = 42;
SELECT id, name FROM users WHERE name = 'Ada';
SELECT * FROM users WHERE id >= 10 AND id <= 20;
```

Limitations:

- Only column-to-literal comparisons are supported.
- `AND` is supported; `OR` and `NOT` are deferred.
- Parentheses are deferred.
- Comparisons must use the column's declared type.
- `NULL` comparison uses `IS NULL` and `IS NOT NULL` only if nullable SQL
  values are enabled; ordinary equality with `NULL` is invalid.
- Functions, arithmetic, expressions, and subqueries are unsupported.

All six comparison operators are supported for INT64, STRING, BYTES, and BOOL.
Integers compare numerically without subtraction overflow. Strings and bytes
compare lexicographically by bytes, without locale collation. Boolean ordering
is `FALSE < TRUE`. Predicate columns, operators, and literal types are validated
before fetching rows, so an invalid query fails even on an empty table or when
another predicate would match no rows.

Repeated bounds are combined independently of their order. Strict bounds are
filtered after inclusive row retrieval, avoiding integer overflow at minimum
and maximum values. Contradictory valid predicates return no rows; they do not
hide invalid predicates elsewhere in the same query.

### Access Path Selection

The current executor chooses these access paths:

1. Primary-key equality uses the table's primary-key lookup.
2. Explicit lower and upper primary-key bounds use the tightest supplied
   inclusive table range, followed by filtering for strict comparisons.
3. Unbounded, one-sided, and primary-key inequality predicates use the full
   table-prefix scan followed by filtering.

Every selected row is checked against all predicates, including predicates used
to choose the access path. Secondary-index selection and efficient one-sided
iterators remain future improvements; the executor does not yet use indexes.

The result must be the same regardless of whether an index or full scan is
used. Index selection is an execution detail and is not visible in query
results.

## Errors

The parser should report syntax errors with a token position. The executor
should distinguish:

- unknown table;
- unknown column;
- duplicate column;
- invalid literal type;
- missing required column;
- duplicate primary key;
- unsupported predicate;
- malformed statement.

Error wording may evolve, but errors must not silently reinterpret invalid
input.

`Execute` also validates directly constructed ASTs. Nil databases, nil statement
pointers, empty projections, mismatched INSERT counts, and literal kinds whose
Go values have the wrong type return errors instead of panicking. Both value
and non-nil pointer forms of supported statement ASTs are accepted.

## Grammar Sketch

This is a parser guide, not a complete formal grammar:

```text
statement       := create_table | insert | select
create_table    := CREATE TABLE ident '(' column_def (',' column_def)* ')' ';'?
column_def      := ident type primary_key? not_null?
insert          := INSERT INTO ident '(' ident (',' ident)* ')' VALUES '(' literal (',' literal)* ')' ';'?
select          := SELECT ('*' | ident (',' ident)*) FROM ident where? ';'?
where           := WHERE predicate (AND predicate)*
predicate       := ident comparison literal
comparison      := '=' | '!=' | '<' | '<=' | '>' | '>='
type            := INT64 | STRING | BYTES | BOOL
literal         := integer | string | TRUE | FALSE
```

## Unsupported Features

The initial SQL layer does not support:

- `UPDATE` and `DELETE` statements;
- `DROP TABLE` or `ALTER TABLE`;
- SQL transaction statements;
- joins;
- aggregation, `GROUP BY`, or `HAVING`;
- `ORDER BY`, `LIMIT`, or `OFFSET`;
- `OR`, `NOT`, nested expressions, or parentheses;
- functions and expressions;
- subqueries and common table expressions;
- views, triggers, or stored procedures;
- composite primary keys;
- automatic index creation;
- query parameters or prepared statements.

These are deferred until the core parser and executor have stable semantics.

## Follow-Up Implementation Tasks

1. Implement a tokenizer for the lexical rules.
2. Define AST types for the supported statements.
3. Implement parser position and syntax errors.
4. Validate statements against loaded table schemas.
5. Execute table creation and insert statements.
6. Execute primary-key, indexed, and full-scan select paths.
7. Add end-to-end tests comparing indexed queries with full scans.
8. Add benchmarks for parse, execution, indexed lookup, and full scans.
