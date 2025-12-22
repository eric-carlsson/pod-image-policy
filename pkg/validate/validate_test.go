package validate

import (
	"testing"

	"github.com/eric-carlsson/pod-image-policy/pkg/config"

	corev1 "k8s.io/api/core/v1"
)

func TestValidatePodImages(t *testing.T) {
	cases := []struct {
		name         string
		pod          corev1.Pod
		cfg          config.ValidateConfig
		wantAllowed  bool
		wantMessage  string
		wantWarnings int
		wantErr      bool
	}{
		{
			name: "deny latest tag",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:latest"}}}},
			cfg: config.ValidateConfig{

				Rules: []config.ValidateRule{{
					Match:   config.ImageMatch{Tag: strPtr("latest")},
					Action:  "deny",
					Message: "Pin image tags (no :latest)",
				}},
			},
			wantAllowed: false,
			wantMessage: "Pin image tags (no :latest)",
		},
		{
			name: "warn for rc tag",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:1.0-rc1"}}}},
			cfg: config.ValidateConfig{

				Rules: []config.ValidateRule{{
					Match:   config.ImageMatch{Tag: strPtr("*rc*")},
					Action:  "warn",
					Message: "Release candidates are discouraged",
				}},
			},
			wantAllowed:  true,
			wantWarnings: 1,
		},
		{
			name: "allow matching specific rule",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "ghcr.io/org/app:1.0"}}}},
			cfg: config.ValidateConfig{

				Rules: []config.ValidateRule{{
					Match:   config.ImageMatch{Registry: strPtr("ghcr.io")},
					Action:  "allow",
					Message: "",
				}},
			},
			wantAllowed: true,
		},
		{
			name: "default policy allow when no match",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:1.0"}}}},
			cfg: config.ValidateConfig{

				Rules: []config.ValidateRule{{
					Match:   config.ImageMatch{Registry: strPtr("untrusted.io")},
					Action:  "deny",
					Message: "Untrusted registry",
				}},
			},
			wantAllowed: true,
		},
		{
			name: "deny by default with catch-all rule",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:1.0"}}}},
			cfg: config.ValidateConfig{
				Rules: []config.ValidateRule{
					{
						Match:   config.ImageMatch{Registry: strPtr("trusted.io")},
						Action:  "allow",
						Message: "",
					},
					{
						Match:   config.ImageMatch{},
						Action:  "deny",
						Message: "Image not from trusted registry",
					},
				},
			},
			wantAllowed: false,
			wantMessage: "Image not from trusted registry",
		},
		{
			name: "first matching rule wins",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:latest"}}}},
			cfg: config.ValidateConfig{

				Rules: []config.ValidateRule{
					{
						Match:   config.ImageMatch{Tag: strPtr("latest")},
						Action:  "deny",
						Message: "No latest",
					},
					{
						Match:   config.ImageMatch{Repository: strPtr("library/nginx")},
						Action:  "allow",
						Message: "",
					},
				},
			},
			wantAllowed: false,
			wantMessage: "No latest",
		},
		{
			name: "all containers validated",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "nginx:1.0"},
					{Image: "redis:latest"},
				},
			}},
			cfg: config.ValidateConfig{
				Rules: []config.ValidateRule{{
					Match:   config.ImageMatch{Tag: strPtr("latest")},
					Action:  "deny",
					Message: "No latest",
				}},
			},
			wantAllowed: false,
			wantMessage: "No latest",
		},
		{
			name: "init containers validated",
			pod:  corev1.Pod{Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Image: "busybox:latest"}}}},
			cfg: config.ValidateConfig{

				Rules: []config.ValidateRule{{
					Match:   config.ImageMatch{Tag: strPtr("latest")},
					Action:  "deny",
					Message: "No latest",
				}},
			},
			wantAllowed: false,
			wantMessage: "No latest",
		},
		{
			name: "ephemeral containers validated",
			pod:  corev1.Pod{Spec: corev1.PodSpec{EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Image: "debug:latest"}}}}},
			cfg: config.ValidateConfig{

				Rules: []config.ValidateRule{{
					Match:   config.ImageMatch{Tag: strPtr("latest")},
					Action:  "deny",
					Message: "No latest",
				}},
			},
			wantAllowed: false,
			wantMessage: "No latest",
		},
		{
			name: "multiple warnings accumulate",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Image: "nginx:1.0-rc1"},
					{Image: "redis:2.0-rc2"},
				},
			}},
			cfg: config.ValidateConfig{
				Rules: []config.ValidateRule{{
					Match:   config.ImageMatch{Tag: strPtr("*rc*")},
					Action:  "warn",
					Message: "RC discouraged",
				}},
			},
			wantAllowed:  true,
			wantWarnings: 2,
		},
		{
			name: "no rules with allow default",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:1.0"}}}},
			cfg: config.ValidateConfig{},
			wantAllowed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			validator, err := NewValidator(tc.cfg)
			if err != nil {
				if tc.wantErr {
					return
				}
				t.Fatalf("compile validate rules: %v", err)
			}

			result, err := validator.ValidatePodImages(&tc.pod)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("validate: %v", err)
			}

			if result.Allowed != tc.wantAllowed {
				t.Fatalf("allowed=%v want %v", result.Allowed, tc.wantAllowed)
			}
			if result.Message != tc.wantMessage {
				t.Fatalf("message=%q want %q", result.Message, tc.wantMessage)
			}
			if len(result.Warnings) != tc.wantWarnings {
				t.Fatalf("warnings count=%d want %d (warnings: %v)", len(result.Warnings), tc.wantWarnings, result.Warnings)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
