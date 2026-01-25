// Package image implements helpers for converting image references to and from strings.
package image

import (
	"strings"

	"github.com/distribution/reference"
)

// Image represents a container image reference.
type Image struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

// Parse an image reference from a string.
func Parse(str string) (Image, error) {
	ref, err := reference.ParseAnyReference(str)
	if err != nil {
		return Image{}, err
	}

	var image Image

	if named, ok := ref.(reference.Named); ok {
		image.Registry = reference.Domain(named)
		image.Repository = reference.Path(named)
	}
	if tagged, ok := ref.(reference.Tagged); ok {
		image.Tag = tagged.Tag()
	}
	if digested, ok := ref.(reference.Digested); ok {
		image.Digest = digested.Digest().String()
	}
	return image, nil
}

// String returns a canonical string representation of the image.
func (i Image) String() string {
	var b strings.Builder
	if i.Registry != "" {
		b.WriteString(i.Registry)
		b.WriteByte('/')
	}
	b.WriteString(i.Repository)
	if i.Tag != "" {
		b.WriteByte(':')
		b.WriteString(i.Tag)
	}
	if i.Digest != "" {
		b.WriteByte('@')
		b.WriteString(i.Digest)
	}
	return b.String()
}
