package image

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Image
		wantErr bool
	}{
		{
			name:  "name only",
			input: "busybox",
			want: Image{
				Registry:   "docker.io",
				Repository: "library/busybox",
			},
		},
		{
			name:  "name with tag",
			input: "alpine:3.18",
			want: Image{
				Registry:   "docker.io",
				Repository: "library/alpine",
				Tag:        "3.18",
			},
		},
		{
			name:  "registry with tag",
			input: "ghcr.io/org/app:1.2.3",
			want: Image{
				Registry:   "ghcr.io",
				Repository: "org/app",
				Tag:        "1.2.3",
			},
		},
		{
			name:  "name with digest",
			input: "alpine@sha256:7dbaa93f0f9e5a3cd5080593d8b8e9b9b7d4f9b0000000000000000000000000",
			want: Image{
				Registry:   "docker.io",
				Repository: "library/alpine",
				Digest:     "sha256:7dbaa93f0f9e5a3cd5080593d8b8e9b9b7d4f9b0000000000000000000000000",
			},
		},
		{
			name:  "registry with tag and digest",
			input: "gcr.io/project/image:latest@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want: Image{
				Registry:   "gcr.io",
				Repository: "project/image",
				Tag:        "latest",
				Digest:     "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		},
		{
			name:    "invalid reference",
			input:   "not a valid reference@@",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestImage_String(t *testing.T) {
	tests := []struct {
		name string
		img  Image
		want string
	}{
		{
			name: "registry and repository only",
			img: Image{
				Registry:   "docker.io",
				Repository: "library/busybox",
			},
			want: "docker.io/library/busybox",
		},
		{
			name: "with tag",
			img: Image{
				Registry:   "docker.io",
				Repository: "library/alpine",
				Tag:        "3.18",
			},
			want: "docker.io/library/alpine:3.18",
		},
		{
			name: "with digest",
			img: Image{
				Registry:   "docker.io",
				Repository: "library/alpine",
				Digest:     "sha256:7dbaa93f0f9e5a3cd5080593d8b8e9b9b7d4f9b0000000000000000000000000",
			},
			want: "docker.io/library/alpine@sha256:7dbaa93f0f9e5a3cd5080593d8b8e9b9b7d4f9b0000000000000000000000000",
		},
		{
			name: "with tag and digest",
			img: Image{
				Registry:   "gcr.io",
				Repository: "project/image",
				Tag:        "latest",
				Digest:     "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			want: "gcr.io/project/image:latest@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		{
			name: "no registry",
			img: Image{
				Repository: "library/busybox",
				Tag:        "1.0",
			},
			want: "library/busybox:1.0",
		},
		{
			name: "only repository",
			img: Image{
				Repository: "library/busybox",
			},
			want: "library/busybox",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.img.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
