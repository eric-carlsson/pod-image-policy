package policy

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/eric-carlsson/pod-image-policy/pkg/image"
	"go.yaml.in/yaml/v4"
)

type Policy struct {
	Mutate   MutatePolicy   `yaml:"mutate"`
	Validate ValidatePolicy `yaml:"validate"`
}

// Parse decodes a policy from a reader.
func Parse(r io.Reader) (*Policy, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var pol Policy
	if err := yaml.Unmarshal(data, &pol); err != nil {
		return nil, err
	}

	return &pol, nil
}

// Load reads and parses a policy file.
func Load(path string) (*Policy, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

type MutatePolicy struct {
	Rules []MutateRule `yaml:"rules"`
}

type MutateRule struct {
	Match   Match   `yaml:"match"`
	Replace Replace `yaml:"replace"`
	Message string  `yaml:"message"`
}

type ValidatePolicy struct {
	Rules []ValidateRule `yaml:"rules"`
}

type ValidateRule struct {
	Match   Match  `yaml:"match"`
	Action  Action `yaml:"action"`
	Message string `yaml:"message"`
}

type MatchExp struct {
	regexp.Regexp
}

func (me *MatchExp) UnmarshalYAML(node *yaml.Node) error {
	pattern := node.Value

	// Automatically anchor the pattern if neither ^ nor $ is present.
	hasStart := strings.HasPrefix(pattern, "^")
	hasEnd := strings.HasSuffix(pattern, "$")

	if !hasStart && !hasEnd {
		pattern = "^" + pattern + "$"
	}

	exp, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("column %v: %w", node.Column, err)
	}
	me.Regexp = *exp
	return nil
}

type Action int

const (
	Allow Action = iota
	Warn
	Deny
)

func (a *Action) UnmarshalYAML(node *yaml.Node) error {
	var val Action
	switch strings.ToLower(strings.TrimSpace(node.Value)) {
	case "allow":
		val = Allow
	case "warn":
		val = Warn
	case "deny":
		val = Deny
	default:
		return fmt.Errorf("column %v: invalid action: %v", node.Column, node.Value)
	}
	*a = val
	return nil
}

type Replace struct {
	Registry   *string `yaml:"registry"`
	Repository *string `yaml:"repository"`
	Tag        *string `yaml:"tag"`
	Digest     *string `yaml:"digest"`
}

type Match struct {
	Registry   *MatchExp `yaml:"registry"`
	Repository *MatchExp `yaml:"repository"`
	Tag        *MatchExp `yaml:"tag"`
	Digest     *MatchExp `yaml:"digest"`
}

// Match reports whether all defined expressions in Match are satisfied by the image fields.
func (m *Match) Match(img image.Image) bool {
	fields := []struct {
		match *MatchExp
		image string
	}{
		{m.Registry, img.Registry},
		{m.Repository, img.Repository},
		{m.Tag, img.Tag},
		{m.Digest, img.Digest},
	}

	for _, field := range fields {
		// Skip if no match expression
		if field.match == nil {
			continue
		}
		if ok := field.match.Match([]byte(field.image)); !ok {
			return false
		}
	}

	return true
}

// MatchAndReplace returns an image with registry, repository, tag, and digest
// replaced per the Replace templates when the match criteria are met, using
// regex captures if present; otherwise it returns the original image.
func (m *Match) MatchAndReplace(img image.Image, replace Replace) (image.Image, error) {
	if !m.Match(img) {
		return img, nil
	}

	result := img

	fields := []struct {
		matchExp   *MatchExp
		replaceStr *string
		imgValue   string
		setResult  func(string)
	}{
		{m.Registry, replace.Registry, img.Registry, func(v string) { result.Registry = v }},
		{m.Repository, replace.Repository, img.Repository, func(v string) { result.Repository = v }},
		{m.Tag, replace.Tag, img.Tag, func(v string) { result.Tag = v }},
		{m.Digest, replace.Digest, img.Digest, func(v string) { result.Digest = v }},
	}

	for _, field := range fields {
		// Skip if no replacement template
		if field.replaceStr == nil {
			continue
		}

		// If there's a match pattern, extract captures for template expansion
		if field.matchExp != nil {
			matches := field.matchExp.FindStringSubmatch(field.imgValue)
			if matches != nil {
				replaced, err := expandTemplate(*field.replaceStr, matches)
				if err != nil {
					return result, fmt.Errorf("%s: %w", field.matchExp.String(), err)
				}
				field.setResult(replaced)
			}
		} else {
			// No match pattern, use replacement as-is
			field.setResult(*field.replaceStr)
		}
	}

	return result, nil
}

// expandTemplate replaces ${1}, ${2}, etc. with regex capture groups.
// Returns an error if the template references captures that don't exist.
func expandTemplate(template string, captures []string) (string, error) {
	result := template

	// Replace ${N} patterns with captured groups
	// captures[0] is the full match, captures[1] is first group, etc.
	for i := 1; i < len(captures); i++ {
		placeholder := fmt.Sprintf("${%d}", i)
		result = strings.ReplaceAll(result, placeholder, captures[i])
	}

	// Check if any unreplaced placeholders remain
	if strings.Contains(result, "${") {
		return "", fmt.Errorf("template '%q' references missing capture groups", template)
	}

	return result, nil
}
