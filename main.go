// Command sql-migration-lint checks SQL migration files for patterns
// that tend to cause trouble in production: destructive statements
// with no existence guard, SELECT *, and statements missing their
// terminating semicolon.
package main

import (
	"encoding/json"
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
	format := flag.String("format", "text", "output format: \"text\" or \"json\"")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: sql-migration-lint [-config file] [-format text|json] <file.sql> [more.sql ...]")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *format != "text" && *format != "json" {
		fmt.Fprintf(os.Stderr, "-format must be \"text\" or \"json\", got %q\n", *format)
		os.Exit(2)
	}

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

	var findings []lint.Finding
	var hasError bool

	for _, path := range args {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			hasError = true
			continue
		}

		for _, f := range linter.LintFile(path, content) {
			findings = append(findings, f)
			if f.Severity == "error" {
				hasError = true
			}
		}
	}

	if *format == "json" {
		printFindingsJSON(findings)
	} else {
		for _, f := range findings {
			printFinding(f)
		}
		if len(findings) > 0 {
			fmt.Printf("\n%d finding(s)\n", len(findings))
		}
	}

	if hasError {
		os.Exit(1)
	}
}

// jsonFinding is the -format=json shape for a single finding. It omits
// SourceLine: a CI tool that wants the source text can read it from the
// file itself at File:Line, and the caret rendering in the text format
// doesn't translate to structured output anyway.
type jsonFinding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// printFindingsJSON writes every finding as a single JSON array to
// stdout, in the same order LintFile produced them (sorted by file
// argument order, then position). An empty slice still marshals to
// "[]" rather than "null" so consumers can always index into it.
func printFindingsJSON(findings []lint.Finding) {
	out := make([]jsonFinding, len(findings))
	for i, f := range findings {
		out[i] = jsonFinding{
			File:     f.File,
			Line:     f.Line,
			Col:      f.Col,
			Rule:     f.Rule,
			Severity: f.Severity,
			Message:  f.Message,
		}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println(string(data))
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
