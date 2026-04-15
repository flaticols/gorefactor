package rules

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Rule denies dependencies from one package pattern to another.
type Rule struct {
	From   string `toml:"from"`
	To     string `toml:"to"`
	Reason string `toml:"reason"`
}

type fileDoc struct {
	Deny []Rule `toml:"deny"`
}

// Parse loads deny rules from a TOML file.
func Parse(path string) ([]Rule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc fileDoc
	if err := toml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	for i, rule := range doc.Deny {
		doc.Deny[i].From = strings.TrimSpace(rule.From)
		doc.Deny[i].To = strings.TrimSpace(rule.To)
		doc.Deny[i].Reason = strings.TrimSpace(rule.Reason)
		if doc.Deny[i].From == "" || doc.Deny[i].To == "" {
			return nil, fmt.Errorf("deny rule %d must set both from and to", i+1)
		}
	}
	return append([]Rule(nil), doc.Deny...), nil
}
