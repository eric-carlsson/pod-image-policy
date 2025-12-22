package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v4"
)

// AdmissionConfig holds mutate/validate policy configuration loaded from YAML.
type AdmissionConfig struct {
	Mutate   MutateConfig   `yaml:"mutate"`
	Validate ValidateConfig `yaml:"validate"`
}

type MutateConfig struct {
	Rules []MutateRule `yaml:"rules"`
}

type MutateRule struct {
	Match   ImageMatch   `yaml:"match"`
	Replace ImageReplace `yaml:"replace"`
	Message string       `yaml:"message"`
}

type ValidateConfig struct {
	Rules []ValidateRule `yaml:"rules"`
}

type ValidateRule struct {
	Match   ImageMatch `yaml:"match"`
	Action  string     `yaml:"action"`
	Message string     `yaml:"message"`
}

type ImageMatch struct {
	Registry   *string `yaml:"registry"`
	Repository *string `yaml:"repository"`
	Tag        *string `yaml:"tag"`
	Digest     *string `yaml:"digest"`
}

type ImageReplace struct {
	Registry   *string `yaml:"registry"`
	Repository *string `yaml:"repository"`
	Tag        *string `yaml:"tag"`
	Digest     *string `yaml:"digest"`
}

// Load reads the YAML config from path (optional), applying defaults.
func Load(path string) (*AdmissionConfig, error) {
	cfg := &AdmissionConfig{}
	Default(cfg)

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	Default(cfg)
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Default fills unset defaults in-place.
func Default(cfg *AdmissionConfig) {
	// Currently no defaults to set
}

// Validate checks configuration consistency.
func Validate(cfg *AdmissionConfig) error {
	if err := validateMutateConfig(cfg.Mutate); err != nil {
		return err
	}
	if err := validateValidateConfig(cfg.Validate); err != nil {
		return err
	}
	return nil
}

func validateMutateConfig(mc MutateConfig) error {
	for i, rule := range mc.Rules {
		if err := validateMutateRule(rule); err != nil {
			return fmt.Errorf("mutate.rules[%d]: %w", i, err)
		}
	}
	return nil
}

func validateMutateRule(rule MutateRule) error {
	fields := []struct {
		match *string
		repl  *string
		name  string
	}{
		{rule.Match.Registry, rule.Replace.Registry, "registry"},
		{rule.Match.Repository, rule.Replace.Repository, "repository"},
		{rule.Match.Tag, rule.Replace.Tag, "tag"},
		{rule.Match.Digest, rule.Replace.Digest, "digest"},
	}

	for _, f := range fields {
		captures := countCaptures(f.match)
		if err := validatePlaceholders(f.repl, captures, f.name); err != nil {
			return err
		}
	}

	return nil
}

func validateValidateConfig(vc ValidateConfig) error {
	for i, rule := range vc.Rules {
		if rule.Action == "" {
			return fmt.Errorf("validate.rules[%d].action is required", i)
		}
		if rule.Action != "allow" && rule.Action != "deny" && rule.Action != "warn" {
			return fmt.Errorf("validate.rules[%d].action must be allow, deny, or warn", i)
		}
		if rule.Action != "allow" && rule.Message == "" {
			return fmt.Errorf("validate.rules[%d].message is required when action is %s", i, rule.Action)
		}
	}

	return nil
}

func countCaptures(pattern *string) int {
	if pattern == nil {
		return 0
	}
	count := 0
	for _, ch := range *pattern {
		if ch == '*' {
			count++
		}
	}
	return count
}

var placeholderRe = regexp.MustCompile(`\{\$(\d+)\}`)

func validatePlaceholders(repl *string, captures int, field string) error {
	if repl == nil {
		return nil
	}

	matches := placeholderRe.FindAllStringSubmatch(*repl, -1)
	for _, m := range matches {
		if len(m) != 2 {
			continue
		}
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			return fmt.Errorf("%s placeholder parse: %w", field, err)
		}
		if idx < 1 || idx > captures {
			return fmt.Errorf("%s placeholder {$%d} exceeds capture count %d", field, idx, captures)
		}
	}

	return nil
}
