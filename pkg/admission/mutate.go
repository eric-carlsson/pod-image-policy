package admission

import (
	"fmt"

	"github.com/eric-carlsson/pod-image-policy/pkg/image"
	"github.com/eric-carlsson/pod-image-policy/pkg/policy"
	corev1 "k8s.io/api/core/v1"
)

// Mutator mutates pod images according to a policy.
type Mutator struct {
	Policy *policy.MutatePolicy
}

// MutationResult contains the outcome of mutating a pod's images.
type MutationResult struct {
	Patches  []Patch  // JSON patches to apply to the pod
	Warnings []string // Warning messages describing the mutations performed
}

// Patch represents a JSON Patch operation (RFC 6902).
type Patch struct {
	Op    string `json:"op"`    // Operation type (e.g., "replace")
	Path  string `json:"path"`  // JSON pointer to the field (e.g., "/spec/containers/0/image")
	Value string `json:"value"` // New value for the field
}

// MutatePodImages applies mutation rules to all container images in a pod.
// It returns a MutationResult that contains eventual warnings and JSON patches.
// The first matching rule for each image is applied. If no rules match, the image is unchanged.
func (m *Mutator) MutatePodImages(pod *corev1.Pod) (MutationResult, error) {
	var patches []Patch
	var warnings []string

	slots := CollectImageSlots(pod)
	for _, slot := range slots {
		newImage, msg, err := m.mutateImage(slot.Image)
		if err != nil {
			return MutationResult{}, fmt.Errorf("%s: %w", slot.Path, err)
		}
		if newImage != slot.Image {
			patches = append(patches, Patch{
				Op:    "replace",
				Path:  slot.Path,
				Value: newImage,
			})
			if msg != "" {
				warnings = append(warnings, fmt.Sprintf("%s: %s -> %s (%s)", slot.Path, slot.Image, newImage, msg))
			}
		}
	}

	return MutationResult{
		Patches:  patches,
		Warnings: warnings,
	}, nil
}

func (m *Mutator) mutateImage(imgStr string) (string, string, error) {
	img, err := image.Parse(imgStr)
	if err != nil {
		return "", "", fmt.Errorf("parse image: %w", err)
	}

	for _, rule := range m.Policy.Rules {
		mutated, err := rule.Match.MatchAndReplace(img, rule.Replace)
		if err != nil {
			return "", "", fmt.Errorf("apply rule: %w", err)
		}

		newImage := mutated.String()
		// Only return if the string representation actually changed. This avoids no-op patches
		// when defaults are added during parsing (docker.io, /library, etc.)
		// It's important that we compare against the canonical form, img.String(), not imgStr directly.
		if newImage != img.String() {
			return newImage, rule.Message, nil
		}
	}

	// No rules matched or no actual change, return original
	return imgStr, "", nil
}
