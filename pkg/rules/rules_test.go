package rules

import (
	"regexp"
	"testing"

	"github.com/eric-carlsson/pod-image-admissiob/pkg/config"
)

func TestRulesMatchAndReplace(t *testing.T) {
	cases := []struct {
		name           string
		rule           config.MutateRule
		registry       string
		repo           string
		tag            string
		digest         string
		wantMatch      bool
		wantRegistry   string
		wantRepo       string
		wantTag        string
		wantDigest     string
		wantApplyErr   bool
		wantRepoIsGlob bool
	}{
		{
			name: "glob capture rewrites repo and tag",
			rule: config.MutateRule{
				Match:   config.ImageMatch{Repository: strPtr("foo/*")},
				Replace: config.ImageReplace{Repository: strPtr("mirror/{$1}"), Tag: strPtr("stable")},
			},
			registry:       "docker.io",
			repo:           "foo/app",
			tag:            "latest",
			wantMatch:      true,
			wantRegistry:   "docker.io",
			wantRepo:       "mirror/app",
			wantTag:        "stable",
			wantDigest:     "",
			wantRepoIsGlob: true,
		},
		{
			name:      "no match different repo",
			rule:      config.MutateRule{Match: config.ImageMatch{Repository: strPtr("foo/*")}},
			registry:  "docker.io",
			repo:      "bar/app",
			tag:       "latest",
			wantMatch: false,
		},
		{
			name: "multiple glob captures",
			rule: config.MutateRule{
				Match:   config.ImageMatch{Repository: strPtr("foo/*/bar/*")},
				Replace: config.ImageReplace{Repository: strPtr("mirror/{$1}/{$2}")},
			},
			registry:       "docker.io",
			repo:           "foo/team/bar/app",
			tag:            "latest",
			wantMatch:      true,
			wantRegistry:   "docker.io",
			wantRepo:       "mirror/team/app",
			wantTag:        "latest",
			wantDigest:     "",
			wantRepoIsGlob: true,
		},
		{
			name:         "exact registry match no change",
			rule:         config.MutateRule{Match: config.ImageMatch{Registry: strPtr("ghcr.io")}},
			registry:     "ghcr.io",
			repo:         "org/app",
			tag:          "1.0",
			wantMatch:    true,
			wantRegistry: "ghcr.io",
			wantRepo:     "org/app",
			wantTag:      "1.0",
			wantDigest:   "",
		},
		{
			name:         "digest preserved when tag replaced",
			rule:         config.MutateRule{Match: config.ImageMatch{}, Replace: config.ImageReplace{Tag: strPtr("new")}},
			registry:     "docker.io",
			repo:         "library/nginx",
			tag:          "old",
			digest:       "sha256:abc",
			wantMatch:    true,
			wantRegistry: "docker.io",
			wantRepo:     "library/nginx",
			wantTag:      "new",
			wantDigest:   "sha256:abc",
		},
		{
			name: "apply errors when capture missing",
			rule: config.MutateRule{
				Match:   config.ImageMatch{Repository: strPtr("foo/*")},
				Replace: config.ImageReplace{Repository: strPtr("mirror/{$2}")},
			},
			registry:       "docker.io",
			repo:           "foo/app",
			tag:            "latest",
			wantMatch:      true,
			wantApplyErr:   true,
			wantRepoIsGlob: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiled, err := CompileRules([]config.MutateRule{tc.rule})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			rule := compiled[0]

			if tc.wantRepoIsGlob && !rule.Match.Repository.isGlob {
				t.Fatalf("expected repository to be compiled as glob")
			}

			matched, captures, err := RuleMatchesCompiled(rule, tc.registry, tc.repo, tc.tag, tc.digest)
			if err != nil {
				t.Fatalf("match: %v", err)
			}
			if matched != tc.wantMatch {
				t.Fatalf("match=%v want %v", matched, tc.wantMatch)
			}
			if !matched {
				return
			}

			gotRegistry, gotRepo, gotTag, gotDigest, err := ApplyReplace(tc.registry, tc.repo, tc.tag, tc.digest, rule.Replace, captures)
			if tc.wantApplyErr {
				if err == nil {
					t.Fatalf("expected apply error")
				}
				return
			}
			if err != nil {
				t.Fatalf("apply: %v", err)
			}

			if gotRegistry != tc.wantRegistry || gotRepo != tc.wantRepo || gotTag != tc.wantTag || gotDigest != tc.wantDigest {
				t.Fatalf("unexpected result: %q %q %q %q", gotRegistry, gotRepo, gotTag, gotDigest)
			}
		})
	}
}

func TestApplyReplaceMissingCapture(t *testing.T) {
	replace := config.ImageReplace{Tag: strPtr("v{$1}")}
	_, _, _, _, err := ApplyReplace("docker.io", "foo/app", "latest", "", replace, nil)
	if err == nil {
		t.Fatalf("expected error when captures missing")
	}
}

func TestGlobToRegex(t *testing.T) {
	re := regexp.MustCompile(globToRegex("foo/*/bar"))
	if !re.MatchString("foo/team/bar") {
		t.Fatalf("expected glob regex to match")
	}
	if re.MatchString("foo/team/other") {
		t.Fatalf("expected glob regex not to match non-bar suffix")
	}
}

func strPtr(s string) *string { return &s }
