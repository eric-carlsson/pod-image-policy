package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/eric-carlsson/pod-image-policy/pkg/config"
)

// Rule holds precompiled match patterns to avoid recompilation per image.
type Rule struct {
	Match   CompiledMatch
	Replace config.ImageReplace
	Message string
}

// CompiledMatch holds precompiled match patterns for all image fields.
type CompiledMatch struct {
	Registry   compiledField
	Repository compiledField
	Tag        compiledField
	Digest     compiledField
}

type compiledField struct {
	pattern *string
	re      *regexp.Regexp
	isGlob  bool
}

// CompileRules precompiles glob patterns for faster matching and reuse.
func CompileRules(rules []config.MutateRule) ([]Rule, error) {
	compiled := make([]Rule, 0, len(rules))
	for i, rule := range rules {
		cm, err := CompileMatch(rule.Match)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		compiled = append(compiled, Rule{Match: cm, Replace: rule.Replace, Message: rule.Message})
	}
	return compiled, nil
}

// CompileMatch compiles a single ImageMatch into a CompiledMatch.
func CompileMatch(m config.ImageMatch) (CompiledMatch, error) {
	var cm CompiledMatch
	var err error

	cm.Registry, err = compileField(m.Registry)
	if err != nil {
		return cm, fmt.Errorf("registry: %w", err)
	}
	cm.Repository, err = compileField(m.Repository)
	if err != nil {
		return cm, fmt.Errorf("repository: %w", err)
	}
	cm.Tag, err = compileField(m.Tag)
	if err != nil {
		return cm, fmt.Errorf("tag: %w", err)
	}
	cm.Digest, err = compileField(m.Digest)
	if err != nil {
		return cm, fmt.Errorf("digest: %w", err)
	}

	return cm, nil
}

func compileField(pattern *string) (compiledField, error) {
	if pattern == nil {
		return compiledField{pattern: nil}, nil
	}

	if *pattern == "" {
		return compiledField{pattern: pattern}, nil
	}

	if !strings.Contains(*pattern, "*") {
		return compiledField{pattern: pattern}, nil
	}

	re, err := regexp.Compile(globToRegex(*pattern))
	if err != nil {
		return compiledField{}, err
	}
	return compiledField{pattern: pattern, re: re, isGlob: true}, nil
}

// RuleMatches matches using precompiled rule patterns.
func RuleMatches(rule Rule, registry, repo, tag, digest string) (bool, map[string][]string, error) {
	captures := make(map[string][]string)

	checks := []struct {
		cf    compiledField
		value string
		key   string
	}{
		{rule.Match.Registry, registry, "registry"},
		{rule.Match.Repository, repo, "repository"},
		{rule.Match.Tag, tag, "tag"},
		{rule.Match.Digest, digest, "digest"},
	}

	for _, check := range checks {
		ok, caps, err := matchField(check.cf, check.value)
		if err != nil {
			return false, nil, err
		}
		if !ok {
			return false, nil, nil
		}
		if len(caps) > 0 {
			captures[check.key] = caps
		}
	}

	return true, captures, nil
}

// ApplyReplace substitutes captures into replace fields and returns updated parts.
func ApplyReplace(registry, repo, tag, digest string, replace config.ImageReplace, captures map[string][]string) (string, string, string, string, error) {
	if replace.Registry != nil {
		v, err := substitutePlaceholders(*replace.Registry, captures["registry"])
		if err != nil {
			return "", "", "", "", err
		}
		registry = v
	}
	if replace.Repository != nil {
		v, err := substitutePlaceholders(*replace.Repository, captures["repository"])
		if err != nil {
			return "", "", "", "", err
		}
		repo = v
	}
	if replace.Tag != nil {
		v, err := substitutePlaceholders(*replace.Tag, captures["tag"])
		if err != nil {
			return "", "", "", "", err
		}
		tag = v
	}
	if replace.Digest != nil {
		v, err := substitutePlaceholders(*replace.Digest, captures["digest"])
		if err != nil {
			return "", "", "", "", err
		}
		digest = v
	}

	return registry, repo, tag, digest, nil
}

func matchField(cf compiledField, value string) (bool, []string, error) {
	if cf.pattern == nil {
		return true, nil, nil
	}

	p := *cf.pattern
	if p == "" {
		return value == "", nil, nil
	}

	if !cf.isGlob {
		return value == p, nil, nil
	}

	matches := cf.re.FindStringSubmatch(value)
	if matches == nil {
		return false, nil, nil
	}
	return true, matches[1:], nil
}

var placeholderRe = regexp.MustCompile(`\{\$(\d+)\}`)

func substitutePlaceholders(template string, captures []string) (string, error) {
	matches := placeholderRe.FindAllStringSubmatch(template, -1)
	if len(matches) > 0 && len(captures) == 0 {
		return "", fmt.Errorf("placeholders present but no captures available")
	}

	for _, m := range matches {
		if len(m) != 2 {
			continue
		}
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			return "", err
		}
		if idx < 1 || idx > len(captures) {
			return "", fmt.Errorf("placeholder {$%d} has no capture", idx)
		}
	}

	replaced := placeholderRe.ReplaceAllStringFunc(template, func(s string) string {
		match := placeholderRe.FindStringSubmatch(s)
		if len(match) != 2 {
			return s
		}
		idx, err := strconv.Atoi(match[1])
		if err != nil {
			return s
		}
		return captures[idx-1]
	})

	return replaced, nil
}

func globToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for _, ch := range pattern {
		if ch == '*' {
			b.WriteString("(.*)")
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(ch)))
	}
	b.WriteString("$")
	return b.String()
}
