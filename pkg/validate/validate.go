package validate

import (
	"fmt"

	"github.com/eric-carlsson/pod-image-policy/pkg/admission"
	"github.com/eric-carlsson/pod-image-policy/pkg/config"
	"github.com/eric-carlsson/pod-image-policy/pkg/image"
	"github.com/eric-carlsson/pod-image-policy/pkg/rules"

	corev1 "k8s.io/api/core/v1"
)

// ValidationResult holds the outcome of validating a pod's images.
type ValidationResult struct {
	Allowed  bool
	Warnings []string
	Message  string
}

// ValidateRule holds a precompiled validation rule.
type ValidateRule struct {
	Match   rules.CompiledMatch
	Action  string
	Message string
}

// Validator caches compiled validation rules for reuse across requests.
type Validator struct {
	rules []ValidateRule
}

// NewValidator compiles validation rules for reuse.
func NewValidator(cfg config.ValidateConfig) (*Validator, error) {
	compiledRules := make([]ValidateRule, 0, len(cfg.Rules))
	for i, rule := range cfg.Rules {
		cm, err := rules.CompileMatch(rule.Match)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		compiledRules = append(compiledRules, ValidateRule{
			Match:   cm,
			Action:  rule.Action,
			Message: rule.Message,
		})
	}
	return &Validator{
		rules: compiledRules,
	}, nil
}

// ValidatePodImages checks all pod container images against validation rules.
func (v *Validator) ValidatePodImages(pod *corev1.Pod) (ValidationResult, error) {
	images := admission.CollectImages(pod)

	var warnings []string

	for _, img := range images {
		action, message, err := v.validateImage(img)
		if err != nil {
			return ValidationResult{}, err
		}

		switch action {
		case "deny":
			return ValidationResult{
				Allowed: false,
				Message: message,
			}, nil
		case "warn":
			warnings = append(warnings, message)
		}
	}

	return ValidationResult{
		Allowed:  true,
		Warnings: warnings,
	}, nil
}

func (v *Validator) validateImage(imageStr string) (string, string, error) {
	named, err := image.ParseNamed(imageStr)
	if err != nil {
		return "", "", err
	}

	registry, repo, tag, digest := image.ExtractParts(named)

	for _, rule := range v.rules {
		matched, _, err := rules.RuleMatches(rules.Rule{Match: rule.Match}, registry, repo, tag, digest)
		if err != nil {
			return "", "", err
		}
		if matched {
			return rule.Action, rule.Message, nil
		}
	}

	// No rule matched, allow by default
	return "allow", "", nil
}
