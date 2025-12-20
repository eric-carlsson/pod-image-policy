package mutate

import (
	"fmt"

	"github.com/eric-carlsson/pod-image-policy/pkg/config"
	"github.com/eric-carlsson/pod-image-policy/pkg/image"
	"github.com/eric-carlsson/pod-image-policy/pkg/rules"

	corev1 "k8s.io/api/core/v1"
)

// PatchOp represents a JSON Patch operation for admission responses.
type PatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// Replace constructs a replace patch operation.
func Replace(path string, value any) PatchOp {
	return PatchOp{Op: "replace", Path: path, Value: value}
}

// Mutator caches compiled mutation rules for reuse across requests.
type Mutator struct {
	rules []rules.CompiledRule
}

// NewMutator compiles mutation rules for reuse.
func NewMutator(cfg config.MutateConfig) (*Mutator, error) {
	if len(cfg.Rules) == 0 {
		return &Mutator{}, nil
	}
	compiledRules, err := rules.CompileRules(cfg.Rules)
	if err != nil {
		return nil, err
	}
	return &Mutator{rules: compiledRules}, nil
}

// RewritePodImages applies cached rules to all pod container images and returns JSON patches.
func (m *Mutator) RewritePodImages(pod *corev1.Pod) ([]PatchOp, error) {
	if len(m.rules) == 0 {
		return nil, nil
	}

	slots := collectImageSlots(pod)
	var patches []PatchOp

	for _, slot := range slots {
		newImage, matched, changed, err := rewriteImage(slot.image, m.rules)
		if err != nil {
			return nil, err
		}

		if !matched {
			continue
		}

		if changed {
			patches = append(patches, Replace(slot.path, newImage))
		}
	}

	return patches, nil
}

type imageSlot struct {
	image string
	path  string
}

func collectImageSlots(pod *corev1.Pod) []imageSlot {
	var slots []imageSlot

	for i, c := range pod.Spec.Containers {
		slots = append(slots, imageSlot{image: c.Image, path: fmt.Sprintf("/spec/containers/%d/image", i)})
	}
	for i, c := range pod.Spec.InitContainers {
		slots = append(slots, imageSlot{image: c.Image, path: fmt.Sprintf("/spec/initContainers/%d/image", i)})
	}
	for i, c := range pod.Spec.EphemeralContainers {
		slots = append(slots, imageSlot{image: c.Image, path: fmt.Sprintf("/spec/ephemeralContainers/%d/image", i)})
	}

	return slots
}

func rewriteImage(imageStr string, compiledRules []rules.CompiledRule) (string, bool, bool, error) {
	named, err := image.ParseNamed(imageStr)
	if err != nil {
		return "", false, false, err
	}

	registry, repo, tag, digest := image.ExtractParts(named)

	for _, rule := range compiledRules {
		matched, captures, err := rules.RuleMatchesCompiled(rule, registry, repo, tag, digest)
		if err != nil {
			return "", false, false, err
		}
		if !matched {
			continue
		}

		newRegistry, newRepo, newTag, newDigest, err := rules.ApplyReplace(registry, repo, tag, digest, rule.Replace, captures)
		if err != nil {
			return "", false, false, err
		}

		newImage, err := image.BuildFromParts(newRegistry, newRepo, newTag, newDigest)
		if err != nil {
			return "", false, false, err
		}
		return newImage, true, newImage != imageStr, nil
	}

	return imageStr, false, false, nil
}
