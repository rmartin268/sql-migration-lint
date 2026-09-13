package lint

import (
	"encoding/json"
	"fmt"
	"os"
)

// RuleConfig overrides the default enabled state and/or severity of
// one rule. A nil Enabled or empty Severity leaves that aspect of the
// rule at its built-in default.
type RuleConfig struct {
	Enabled  *bool  `json:"enabled"`
	Severity string `json:"severity"`
}

// Config is the on-disk shape of a linter config file: a map from rule
// name to the overrides for that rule. A rule with no entry runs with
// its built-in default enabled state and severity.
type Config struct {
	Rules map[string]RuleConfig `json:"rules"`
}

// LoadConfig reads and parses the config file at path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	for name, rc := range cfg.Rules {
		switch rc.Severity {
		case "", "error", "warning":
		default:
			return nil, fmt.Errorf("%s: rule %q: severity must be \"error\" or \"warning\", got %q", path, name, rc.Severity)
		}
	}

	return &cfg, nil
}

// Apply filters and overrides l.Rules according to cfg: a rule
// explicitly disabled is dropped, and a rule with a configured
// severity has that severity forced onto every finding it reports,
// regardless of what the rule's check function set. It returns an
// error if cfg names a rule that isn't registered on l, which is
// almost always a typo in the config file.
func (l *Linter) Apply(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	known := make(map[string]bool, len(l.Rules))
	for _, r := range l.Rules {
		known[r.Name] = true
	}
	for name := range cfg.Rules {
		if !known[name] {
			return fmt.Errorf("config: unknown rule %q", name)
		}
	}

	var kept []Rule
	for _, rule := range l.Rules {
		rc, ok := cfg.Rules[rule.Name]
		if !ok {
			kept = append(kept, rule)
			continue
		}
		if rc.Enabled != nil && !*rc.Enabled {
			continue
		}
		if rc.Severity != "" {
			rule.Check = withSeverity(rule.Check, rc.Severity)
		}
		kept = append(kept, rule)
	}
	l.Rules = kept

	return nil
}

// withSeverity wraps a rule's check function so every finding it
// returns has its severity forced to severity.
func withSeverity(check func(content []byte, stmt Statement, pos *Positions) []Finding, severity string) func(content []byte, stmt Statement, pos *Positions) []Finding {
	return func(content []byte, stmt Statement, pos *Positions) []Finding {
		findings := check(content, stmt, pos)
		for i := range findings {
			findings[i].Severity = severity
		}
		return findings
	}
}
