package policy

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/eric-carlsson/pod-image-policy/pkg/image"
)

func mexp(exp string) *MatchExp {
	return &MatchExp{Regexp: *regexp.MustCompile(exp)}
}

func ptr(s string) *string { return &s }

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError bool
		errorMsg  string
	}{
		{
			name: "successful loading with valid policy",
			content: `mutate:
  rules:
    - match:
        registry: "^old-registry\\.com$"
      replace:
        registry: "new-registry.com"
      message: "migrating to new registry"
validate:
  rules:
    - match:
        repository: "^unsafe/.*$"
      action: "Deny "
      message: "unsafe images not allowed"
`,
			wantError: false,
		},
		{
			name: "successful loading with empty rules",
			content: `mutate:
  rules: []
validate:
  rules: []
`,
			wantError: false,
		},
		{
			name: "invalid action value",
			content: `validate:
  rules:
    - match:
        registry: ".*"
      action: invalid_action
      message: "test"
`,
			wantError: true,
			errorMsg:  "invalid action",
		},
		{
			name: "invalid regex pattern",
			content: `mutate:
  rules:
    - match:
        registry: "^(unclosed"
      replace:
        registry: "new"
`,
			wantError: true,
			errorMsg:  "error parsing regexp",
		},
		{
			name: "malformed yaml",
			content: `mutate:
  rules:
    - match
      registry: invalid
`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := Parse(strings.NewReader(tt.content))

			if tt.wantError {
				if err == nil {
					t.Errorf("Parse() expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Parse() error = %v, expected to contain %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Parse() unexpected error: %v", err)
				}
				if policy == nil {
					t.Error("Parse() returned nil policy")
				}
			}
		})
	}
}

func TestMatch_Match(t *testing.T) {
	tests := []struct {
		name   string
		match  Match
		img    image.Image
		expect bool
	}{
		{
			name: "all fields match",
			match: Match{
				Registry:   mexp("^registry\\.io$"),
				Repository: mexp("^repo/.*$"),
				Tag:        mexp("^v1\\.0\\.0$"),
				Digest:     mexp("^sha256:.*"),
			},
			img: image.Image{
				Registry:   "registry.io",
				Repository: "repo/app",
				Tag:        "v1.0.0",
				Digest:     "sha256:abcd",
			},
			expect: true,
		},
		{
			name: "repository mismatch",
			match: Match{
				Registry:   mexp("^registry\\.io$"),
				Repository: mexp("^other/.*$"),
			},
			img: image.Image{
				Registry:   "registry.io",
				Repository: "repo/app",
			},
			expect: false,
		},
		{
			name: "empty match field is skipped",
			match: Match{
				Registry:   mexp("^registry\\.io$"),
				Repository: mexp("^repo/.*$"),
			},
			img: image.Image{
				Registry:   "registry.io",
				Repository: "repo/app",
				Tag:        "v1",
			},
			expect: true,
		},
		{
			name:   "no matchers and empty image",
			match:  Match{},
			img:    image.Image{},
			expect: true,
		},
		{
			name: "digest mismatch",
			match: Match{
				Digest: mexp("^sha256:1234$"),
			},
			img: image.Image{
				Digest: "sha256:abcd",
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.match.Match(tt.img); got != tt.expect {
				t.Fatalf("Matches() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestMatch_MatchAndReplace(t *testing.T) {
	tests := []struct {
		name      string
		match     Match
		replace   Replace
		img       image.Image
		want      image.Image
		wantError bool
	}{
		{
			name: "replaces with captures",
			match: Match{
				Registry:   mexp("^old\\.example\\.com$"),
				Repository: mexp("^team/(.*)$"),
				Tag:        mexp("^v(\\d+)\\.(\\d+)$"),
			},
			replace: Replace{
				Registry:   ptr("new.example.com"),
				Repository: ptr("project/${1}"),
				Tag:        ptr("release-${1}.${2}"),
			},
			img: image.Image{
				Registry:   "old.example.com",
				Repository: "team/app",
				Tag:        "v1.2",
			},
			want: image.Image{
				Registry:   "new.example.com",
				Repository: "project/app",
				Tag:        "release-1.2",
			},
		},
		{
			name: "no match returns original",
			match: Match{
				Registry: mexp("^other\\.example\\.com$"),
			},
			replace: Replace{
				Registry: ptr("ignored.example.com"),
			},
			img: image.Image{
				Registry: "old.example.com",
			},
			want: image.Image{
				Registry: "old.example.com",
			},
		},
		{
			name: "replace without match exp uses literal",
			match: Match{
				Registry: mexp("^any$"),
			},
			replace: Replace{
				Tag: ptr("latest"),
			},
			img: image.Image{
				Registry: "any",
				Tag:      "v1",
			},
			want: image.Image{
				Registry: "any",
				Tag:      "latest",
			},
		},
		{
			name: "capture missing leaves field unchanged",
			match: Match{
				Tag: mexp("^v(\\d+)$"),
			},
			replace: Replace{
				Tag: ptr("v${2}"),
			},
			img: image.Image{
				Tag: "v1",
			},
			wantError: true,
		},
		{
			name: "partial replacements do not affect others",
			match: Match{
				Registry: mexp("^r1$"),
				Tag:      mexp("^t1$"),
			},
			replace: Replace{
				Registry: ptr("r2"),
			},
			img: image.Image{
				Registry:   "r1",
				Repository: "keep/me",
				Tag:        "t1",
			},
			want: image.Image{
				Registry:   "r2",
				Repository: "keep/me",
				Tag:        "t1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.match.MatchAndReplace(tt.img, tt.replace)

			if tt.wantError {
				if err == nil {
					t.Errorf("MatchAndReplace() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("MatchAndReplace() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("MatchAndReplace() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
