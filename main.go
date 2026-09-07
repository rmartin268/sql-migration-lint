// Command sql-migration-lint checks SQL migration files for patterns
// that tend to cause trouble in production: destructive statements
// with no existence guard, SELECT *, and statements missing their
// terminating semicolon.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rmartin268/sql-migration-lint/lint"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: sql-migration-lint <file.sql> [more.sql ...]")
		os.Exit(2)
	}

	linter := lint.NewLinter()

	var total int
	var hasError bool

	for _, path := range os.Args[1:] {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			hasError = true
			continue
		}

		for _, f := range linter.LintFile(path, content) {
			printFinding(f)
			total++
			if f.Severity == "error" {
				hasError = true
			}
		}
	}

	if total > 0 {
		fmt.Printf("\n%d finding(s)\n", total)
	}
	if hasError {
		os.Exit(1)
	}
}

// printFinding renders a finding compiler-style: a summary line
// followed by the offending source line and a caret under the exact
// column the finding points at.
func printFinding(f lint.Finding) {
	fmt.Printf("%s:%d:%d: %s: %s [%s]\n", f.File, f.Line, f.Col, f.Severity, f.Message, f.Rule)

	lineNum := fmt.Sprintf("%d", f.Line)
	gutter := strings.Repeat(" ", len(lineNum))

	fmt.Printf("  %s | %s\n", lineNum, f.SourceLine)
	fmt.Printf("  %s | %s^\n", gutter, strings.Repeat(" ", max(f.Col-1, 0)))
}
