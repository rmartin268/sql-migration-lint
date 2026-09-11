package lint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Finding is a single problem reported against a specific location in
// a migration file.
type Finding struct {
	File       string
	Line, Col  int
	Rule       string
	Severity   string // "error" or "warning"
	Message    string
	SourceLine string
}

// Rule inspects one statement at a time and reports zero or more
// findings. Check is expected to set Line, Col, Severity and Message
// on each returned Finding; the Linter fills in File, Rule and
// SourceLine.
type Rule struct {
	Name  string
	Check func(content []byte, stmt Statement, pos *Positions) []Finding
}

// Linter runs a fixed set of rules over migration files.
type Linter struct {
	Rules []Rule
}

// NewLinter returns a Linter with the built-in rule set enabled.
func NewLinter() *Linter {
	return &Linter{
		Rules: []Rule{
			{Name: "drop-without-guard", Check: checkDropWithoutGuard},
			{Name: "select-star", Check: checkSelectStar},
			{Name: "not-null-no-default", Check: checkNotNullNoDefault},
			{Name: "missing-semicolon", Check: checkMissingSemicolon},
		},
	}
}

// LintFile runs every rule against content, statement by statement,
// and returns all findings sorted by position.
func (l *Linter) LintFile(path string, content []byte) []Finding {
	pos := NewPositions(content)
	stmts := SplitStatements(content)

	var findings []Finding
	for _, stmt := range stmts {
		for _, rule := range l.Rules {
			for _, f := range rule.Check(content, stmt, pos) {
				f.File = path
				f.Rule = rule.Name
				f.SourceLine = pos.Line(f.Line)
				findings = append(findings, f)
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Col < findings[j].Col
	})

	return findings
}

var dropRe = regexp.MustCompile(`(?i)\bDROP\s+(TABLE|COLUMN)\s+(IF\s+EXISTS\s+)?`)

// checkDropWithoutGuard flags DROP TABLE / DROP COLUMN statements that
// have no IF EXISTS guard, since re-running such a migration after a
// partial failure aborts instead of being a no-op.
func checkDropWithoutGuard(content []byte, stmt Statement, pos *Positions) []Finding {
	text := content[stmt.Start:stmt.End]

	var findings []Finding
	for _, m := range dropRe.FindAllSubmatchIndex(text, -1) {
		if m[4] != -1 {
			// "IF EXISTS" was present (submatch 2 matched).
			continue
		}
		kind := string(text[m[2]:m[3]])
		line, col := pos.LineCol(stmt.Start + m[0])
		findings = append(findings, Finding{
			Line:     line,
			Col:      col,
			Severity: "warning",
			Message:  fmt.Sprintf("DROP %s without IF EXISTS is not idempotent; a retry after a partial failure will abort", kind),
		})
	}
	return findings
}

var selectStarRe = regexp.MustCompile(`(?i)\bSELECT\s+\*\s+FROM\b`)

// checkSelectStar flags SELECT * FROM, which silently changes shape
// when columns are added, removed or reordered later.
func checkSelectStar(content []byte, stmt Statement, pos *Positions) []Finding {
	text := content[stmt.Start:stmt.End]

	var findings []Finding
	for _, m := range selectStarRe.FindAllIndex(text, -1) {
		line, col := pos.LineCol(stmt.Start + m[0])
		findings = append(findings, Finding{
			Line:     line,
			Col:      col,
			Severity: "warning",
			Message:  "SELECT * will silently change shape if columns are added, dropped or reordered; list columns explicitly",
		})
	}
	return findings
}

var (
	alterTableRe = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+\S+\s+`)
	addColumnRe  = regexp.MustCompile(`(?is)^\s*ADD\s+(?:COLUMN\s+)?(\S+)\s+(.*)$`)
	notNullRe    = regexp.MustCompile(`(?i)\bNOT\s+NULL\b`)
	defaultRe    = regexp.MustCompile(`(?i)\bDEFAULT\b`)
)

// nonColumnAddKeywords are the words that can follow ADD in an ALTER
// TABLE action without a COLUMN keyword in front of them, none of
// which name a column being added.
var nonColumnAddKeywords = map[string]bool{
	"CONSTRAINT": true,
	"PRIMARY":    true,
	"FOREIGN":    true,
	"UNIQUE":     true,
	"CHECK":      true,
}

// checkNotNullNoDefault flags ALTER TABLE ... ADD COLUMN ... NOT NULL
// with no DEFAULT. On a table that already has rows this forces the
// database to rewrite (or at least fully validate) every existing row
// while holding a lock, which on a large table can stall the whole
// migration and everything waiting behind it.
func checkNotNullNoDefault(content []byte, stmt Statement, pos *Positions) []Finding {
	text := content[stmt.Start:stmt.End]

	m := alterTableRe.FindIndex(text)
	if m == nil {
		return nil
	}
	actionsStart := m[1]

	var findings []Finding
	for _, chunk := range splitTopLevel(text[actionsStart:], ',') {
		clause := text[actionsStart+chunk[0] : actionsStart+chunk[1]]

		cm := addColumnRe.FindSubmatchIndex(clause)
		if cm == nil {
			continue
		}
		colName := string(clause[cm[2]:cm[3]])
		if nonColumnAddKeywords[strings.ToUpper(colName)] {
			continue
		}

		if defaultRe.Match(clause) {
			continue
		}
		nn := notNullRe.FindIndex(clause)
		if nn == nil {
			continue
		}

		offset := stmt.Start + actionsStart + chunk[0] + nn[0]
		line, col := pos.LineCol(offset)
		findings = append(findings, Finding{
			Line:     line,
			Col:      col,
			Severity: "warning",
			Message:  fmt.Sprintf("column %q is NOT NULL with no DEFAULT; on a populated table this locks it for a full rewrite/validation", colName),
		})
	}
	return findings
}

// checkMissingSemicolon flags a trailing statement that has real SQL
// content but was never terminated with a semicolon.
func checkMissingSemicolon(content []byte, stmt Statement, pos *Positions) []Finding {
	if stmt.Terminated {
		return nil
	}

	text := stmt.Text(content)
	if strings.TrimSpace(stripSQLComments(text)) == "" {
		return nil
	}

	trimmed := strings.TrimRight(text, " \t\r\n")
	offset := stmt.Start + len(trimmed)
	line, col := pos.LineCol(offset)

	return []Finding{{
		Line:     line,
		Col:      col,
		Severity: "error",
		Message:  "statement is not terminated with a semicolon",
	}}
}
