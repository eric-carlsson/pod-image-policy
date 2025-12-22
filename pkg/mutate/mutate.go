package mutate

import (
	"fmt"

	"github.com/eric-carlsson/pod-image-policy/pkg/admission"
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
	rules []rules.Rule
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

// RewritePodImages applies cached rules to all pod container images and returns JSON patches and warnings.
func (m *Mutator) RewritePodImages(pod *corev1.Pod) ([]PatchOp, []string, error) {
	if len(m.rules) == 0 {
		return nil, nil, nil
	}

	slots := admission.CollectImageSlots(pod)
	var patches []PatchOp
	var warnings []string

	for _, slot := range slots {
		newImage, matched, changed, message, err := rewriteImage(slot.Image, m.rules)
		if err != nil {
			return nil, nil, err
		}

		if !matched {
			continue
		}

		if changed {
			patches = append(patches, Replace(slot.Path, newImage))
			if message != "" {
				warnings = append(warnings, fmt.Sprintf("%s: %s -> %s", message, slot.Image, newImage))
			}
		}
	}

	return patches, warnings, nil
}

func rewriteImage(imageStr string, compiledRules []rules.Rule) (string, bool, bool, string, error) {
	named, err := image.ParseNamed(imageStr)
	if err != nil {
		return "", false, false, "", err
	}

	registry, repo, tag, digest := image.ExtractParts(named)

	for _, rule := range compiledRules {
		matched, captures, err := rules.RuleMatches(rule, registry, repo, tag, digest)
		if err != nil {
			return "", false, false, "", err
		}
		if !matched {
			continue
		}

		newRegistry, newRepo, newTag, newDigest, err := rules.ApplyReplace(registry, repo, tag, digest, rule.Replace, captures)
		if err != nil {
			return "", false, false, "", err
		}

		newImage, err := image.BuildFromParts(newRegistry, newRepo, newTag, newDigest)
		if err != nil {
			return "", false, false, "", err
		}
		return newImage, true, newImage != imageStr, rule.Message, nil
	}

	return imageStr, false, false, "", nil
}
