// Package lint implements static checks over SQL migration files.
package lint

import (
	"bytes"
	"strings"
)

// Statement is a single top-level SQL statement, given as a byte range
// into the file it came from. Start points at the first non-whitespace
// byte of the statement (which may be a leading comment); End points
// just past the terminating semicolon, or at end-of-file if the
// statement was never terminated.
type Statement struct {
	Start, End int
	Terminated bool
}

// Text returns the statement's raw source text.
func (s Statement) Text(src []byte) string {
	return string(src[s.Start:s.End])
}

// SplitStatements walks src and breaks it into top-level statements,
// splitting on semicolons that are not inside a string literal, a
// quoted identifier, or a comment.
func SplitStatements(src []byte) []Statement {
	var stmts []Statement
	n := len(src)
	stmtStart := -1
	i := 0

	for i < n {
		c := src[i]

		if isSpace(c) && stmtStart == -1 {
			i++
			continue
		}
		if stmtStart == -1 {
			stmtStart = i
		}

		switch {
		case c == '-' && i+1 < n && src[i+1] == '-':
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			i = j

		case c == '/' && i+1 < n && src[i+1] == '*':
			j := i + 2
			for j+1 < n && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			i = j + 2
			if i > n {
				i = n
			}

		case c == '\'':
			i = scanQuoted(src, i, '\'')

		case c == '"':
			i = scanQuoted(src, i, '"')

		case c == ';':
			stmts = append(stmts, Statement{Start: stmtStart, End: i + 1, Terminated: true})
			stmtStart = -1
			i++

		default:
			i++
		}
	}

	if stmtStart != -1 {
		stmts = append(stmts, Statement{Start: stmtStart, End: n, Terminated: false})
	}

	return stmts
}

// scanQuoted returns the index just past the closing quote of a quoted
// run starting at src[start], treating a doubled quote ('' or "") as an
// escaped literal quote rather than the end of the run.
func scanQuoted(src []byte, start int, quote byte) int {
	n := len(src)
	j := start + 1
	for j < n {
		if src[j] == quote {
			if j+1 < n && src[j+1] == quote {
				j += 2
				continue
			}
			return j + 1
		}
		j++
	}
	return n
}

// stripSQLComments removes -- line comments and /* */ block comments
// from s, used to tell whether a trailing, unterminated statement is
// actually just a trailing comment rather than real SQL.
func stripSQLComments(s string) string {
	var b strings.Builder
	n := len(s)
	i := 0
	for i < n {
		if i+1 < n && s[i] == '-' && s[i+1] == '-' {
			for i < n && s[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < n && s[i] == '/' && s[i+1] == '*' {
			i += 2
			for i+1 < n && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i += 2
			if i > n {
				i = n
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// Positions converts byte offsets into a file into 1-indexed line and
// column numbers, and can recover the source text of a given line for
// printing in diagnostics.
type Positions struct {
	src        []byte
	lineStarts []int
}

// NewPositions indexes the start-of-line offsets in src.
func NewPositions(src []byte) *Positions {
	starts := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &Positions{src: src, lineStarts: starts}
}

// LineCol returns the 1-indexed line and column of the given byte
// offset into the file that was passed to NewPositions.
func (p *Positions) LineCol(offset int) (line, col int) {
	lo, hi := 0, len(p.lineStarts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if p.lineStarts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1, offset - p.lineStarts[lo] + 1
}

// Line returns the source text of the given 1-indexed line, without
// its trailing newline.
func (p *Positions) Line(line int) string {
	if line < 1 || line > len(p.lineStarts) {
		return ""
	}
	start := p.lineStarts[line-1]
	end := len(p.src)
	if line < len(p.lineStarts) {
		end = p.lineStarts[line] - 1
	}
	return string(bytes.TrimRight(p.src[start:end], "\r"))
}
