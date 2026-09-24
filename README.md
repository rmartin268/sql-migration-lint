# sql-migration-lint

A linter for SQL migration files. It catches the handful of mistakes
that don't show up until a migration runs against a real database:
dropping something without an existence guard, `SELECT *` baked into
a view or function, a statement missing its semicolon so it silently
merges into the next one.

Every finding points at an exact line and column, with the source
line and a caret, the way a compiler error does. That's the point of
the tool: "there's a problem in this file" is not actionable at 2am
during a deploy, "line 12, column 1" is.

## Usage

```
go run . migrations/0007_drop_legacy_columns.sql
```

Given this file (note the missing semicolon on the last line — that
matters, see below):

```sql
-- 0007_drop_legacy_columns.sql
ALTER TABLE accounts DROP COLUMN legacy_id;

DROP TABLE staging_accounts;

SELECT * FROM accounts WHERE legacy_id IS NOT NULL
```

it reports:

```
migrations/0007_drop_legacy_columns.sql:2:22: warning: DROP COLUMN without IF EXISTS is not idempotent; a retry after a partial failure will abort [drop-without-guard]
  2 | ALTER TABLE accounts DROP COLUMN legacy_id;
    |                     ^

migrations/0007_drop_legacy_columns.sql:4:1: warning: DROP TABLE without IF EXISTS is not idempotent; a retry after a partial failure will abort [drop-without-guard]
  4 | DROP TABLE staging_accounts;
    | ^

migrations/0007_drop_legacy_columns.sql:6:1: warning: SELECT * will silently change shape if columns are added, dropped or reordered; list columns explicitly [select-star]
  6 | SELECT * FROM accounts WHERE legacy_id IS NOT NULL
    | ^

migrations/0007_drop_legacy_columns.sql:6:51: error: statement is not terminated with a semicolon [missing-semicolon]
  6 | SELECT * FROM accounts WHERE legacy_id IS NOT NULL
    |                                                  ^

4 finding(s)
```

The statement splitter only breaks on semicolons, so if that last line
had merged into the next statement instead of ending the file, its
missing semicolon would just silently fold it into whatever came
after — which is exactly the kind of bug worth catching.

The process exits non-zero if any finding is an error, so it's usable
as a CI gate: `sql-migration-lint migrations/*.sql`.

Pass `-format=json` for machine-readable output instead of the text
report above: a single JSON array of findings (`file`, `line`, `col`,
`rule`, `severity`, `message`), with no summary line mixed in. Useful
when something downstream is going to parse the output rather than a
person reading it in a terminal.

```
sql-migration-lint -format=json migrations/*.sql
```

```json
[
  {
    "file": "migrations/0007_drop_legacy_columns.sql",
    "line": 2,
    "col": 22,
    "rule": "drop-without-guard",
    "severity": "warning",
    "message": "DROP COLUMN without IF EXISTS is not idempotent; a retry after a partial failure will abort"
  }
]
```

## Current checks

- `drop-without-guard` — `DROP TABLE` / `DROP COLUMN` without
  `IF EXISTS`.
- `select-star` — `SELECT * FROM`.
- `not-null-no-default` — `ALTER TABLE ... ADD COLUMN ... NOT NULL`
  with no `DEFAULT`. On a table that already has rows, the database
  has to rewrite or validate every existing row while holding a lock
  to add that column, and there's no default value to backfill them
  with, so the migration either fails outright or stalls everything
  waiting on that lock.
- `missing-semicolon` — a statement that runs to end of file without a
  terminating `;`.

The statement splitter understands `--` and `/* */` comments and
single/double-quoted strings, so semicolons inside a string literal or
a comment don't confuse it.

## Config

By default every rule above runs at its built-in severity. To disable
a rule or change its severity, drop a `.sql-migration-lint.json` file
in the directory you run the linter from, or point at one explicitly
with `-config`:

```json
{
  "rules": {
    "select-star": {"enabled": false},
    "not-null-no-default": {"severity": "error"}
  }
}
```

`enabled: false` drops the rule entirely; `severity` forces every
finding from that rule to `"error"` or `"warning"` regardless of what
the rule would normally report. A rule name in the config file that
the linter doesn't recognize is treated as a mistake and fails the
run rather than being silently ignored.

## Design

There's no SQL parser here on purpose. `lint/scan.go` only needs to
split a file into statement-sized chunks and know the byte offset of
each one; `lint/lint.go` runs regexes over each chunk and converts
match offsets back to line/column with `Positions`, which precomputes
line-start offsets so lookup is a binary search. Good enough for
pattern-based checks, and it means adding a rule is just adding
another regex and a message.

## Status

Early. No dependencies, standard library only. See the checks above
for what's implemented, and Config above for turning individual rules
off or changing their severity. Still explicit-file-args only, no
directory recursion or globbing yet.
