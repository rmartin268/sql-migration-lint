// Command sql-migration-lint checks SQL migration files for patterns
// that tend to cause trouble in production: destructive statements
// with no existence guard, SELECT *, and statements missing their
// terminating semicolon.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rmartin268/sql-migration-lint/lint"
)

// defaultConfigPath is loaded automatically when present and -config
// wasn't given, so a repo can drop in a config file without every
// invocation needing to name it explicitly.
const defaultConfigPath = ".sql-migration-lint.json"

func main() {
	configPath := flag.String("config", "", "path to a rule config file (default: "+defaultConfigPath+" in the current directory, if present)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: sql-migration-lint [-config file] <file.sql> [more.sql ...]")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(2)
	}

	linter := lint.NewLinter()

	path, explicit := *configPath, *configPath != ""
	if !explicit {
		path = defaultConfigPath
	}
	if _, err := os.Stat(path); explicit || err == nil {
		cfg, err := lint.LoadConfig(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := linter.Apply(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	var total int
	var hasError bool

	for _, path := range args {
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
