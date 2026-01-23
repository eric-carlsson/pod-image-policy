package admission

import (
	"fmt"

	"github.com/eric-carlsson/pod-image-policy/pkg/image"
	"github.com/eric-carlsson/pod-image-policy/pkg/policy"
	corev1 "k8s.io/api/core/v1"
)

// Validator validates pod images against a policy.
type Validator struct {
	Policy *policy.ValidatePolicy
}

// ValidationResult contains the outcome of validating a pod's images.
type ValidationResult struct {
	Allowed  bool     // Whether the pod is allowed
	Warnings []string // Warning messages for images that matched warn rules
	Message  string   // Denial message if the pod was denied
}

// ValidatePodImages validates all container images in a pod against the validation policy.
// It returns a ValidationResult indicating whether the pod is allowed, along with any warnings.
// If any image matches a deny rule, the pod is denied immediately with the rule's message.
// Warn rules accumulate warnings that are returned even if the pod is allowed.
// If no rules match, the pod is allowed by default.
func (v *Validator) ValidatePodImages(pod *corev1.Pod) (ValidationResult, error) {
	slots := CollectImageSlots(pod)

	var warnings []string

	for _, slot := range slots {
		img, err := image.Parse(slot.Image)
		if err != nil {
			return ValidationResult{}, fmt.Errorf("parse image: %w", err)
		}

		for _, rule := range v.Policy.Rules {
			if matches := rule.Match.Match(img); matches {
				switch rule.Action {
				case policy.Deny:
					return ValidationResult{
						Allowed: false,
						Message: fmt.Sprintf("%s: %s (%s)", slot.Path, slot.Image, rule.Message),
					}, nil
				case policy.Warn:
					warnings = append(warnings, fmt.Sprintf("%s: %s (%s)", slot.Path, slot.Image, rule.Message))
				case policy.Allow:
					// Explicitly allowed by this rule, but continue checking other rules
				}
			}
		}
	}

	// Default to allowed
	return ValidationResult{
		Allowed:  true,
		Warnings: warnings,
	}, nil
}
