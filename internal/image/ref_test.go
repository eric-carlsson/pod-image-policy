package image

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		name         string
		image        string
		wantRegistry string
		wantRepo     string
		wantTag      string
		wantDigest   string
		wantErr      bool
	}{
		{name: "just name", image: "nginx", wantRegistry: "docker.io", wantRepo: "library/nginx"},
		{name: "namespace and name", image: "library/ubuntu", wantRegistry: "docker.io", wantRepo: "library/ubuntu"},
		{name: "nested repo", image: "foo/bar/baz", wantRegistry: "docker.io", wantRepo: "foo/bar/baz"},
		{name: "explicit registry", image: "ghcr.io/org/app:1.2", wantRegistry: "ghcr.io", wantRepo: "org/app", wantTag: "1.2"},
		{name: "localhost registry", image: "localhost/org/app", wantRegistry: "localhost", wantRepo: "org/app"},
		{name: "registry with port", image: "registry:5000/app", wantRegistry: "registry:5000", wantRepo: "app"},
		{name: "digest only", image: "alpine@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantRegistry: "docker.io", wantRepo: "library/alpine", wantDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{name: "missing repo", image: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			named, err := ParseNamed(tc.image)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			registry, repo, tag, digest := ExtractParts(named)
			if registry != tc.wantRegistry || repo != tc.wantRepo || tag != tc.wantTag || digest != tc.wantDigest {
				t.Fatalf("got registry=%q repo=%q tag=%q digest=%q, want registry=%q repo=%q tag=%q digest=%q", registry, repo, tag, digest, tc.wantRegistry, tc.wantRepo, tc.wantTag, tc.wantDigest)
			}
		})
	}
}

func TestBuildRoundTrip(t *testing.T) {
	orig := "ghcr.io/org/app:1.2.3"
	named, err := ParseNamed(orig)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	registry, repo, tag, digest := ExtractParts(named)
	out, err := BuildFromParts(registry, repo, tag, digest)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if out != orig {
		t.Fatalf("round-trip mismatch: got %q, want %q", out, orig)
	}
}
