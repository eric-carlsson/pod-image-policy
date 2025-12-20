package image

import (
	"fmt"

	"github.com/distribution/reference"
)

// ParseNamed parses an image string into a Named reference (Docker-style normalization).
func ParseNamed(image string) (reference.Named, error) {
	ref, err := reference.ParseAnyReference(image)
	if err != nil {
		return nil, err
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return nil, fmt.Errorf("reference is not named")
	}
	return named, nil
}

// ExtractParts pulls registry/repo/tag/digest strings from a Named reference.
func ExtractParts(named reference.Named) (registry, repo, tag, digest string) {
	registry = reference.Domain(named)
	repo = reference.Path(named)
	if tagged, ok := named.(reference.Tagged); ok {
		tag = tagged.Tag()
	}
	if digested, ok := named.(reference.Digested); ok {
		digest = digested.Digest().String()
	}
	return
}

// BuildFromParts assembles an image string from components and validates via distribution/reference.
func BuildFromParts(registry, repo, tag, digest string) (string, error) {
	base := repo
	if registry != "" {
		base = registry + "/" + repo
	}
	if tag != "" {
		base += ":" + tag
	}
	if digest != "" {
		base += "@" + digest
	}
	if _, err := reference.ParseAnyReference(base); err != nil {
		return "", err
	}
	return base, nil
}
