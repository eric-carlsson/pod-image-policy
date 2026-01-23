package admission

import (
	"regexp"
	"testing"

	"github.com/eric-carlsson/pod-image-policy/pkg/policy"
	corev1 "k8s.io/api/core/v1"
)

func mexp(pattern string) *policy.MatchExp {
	return &policy.MatchExp{Regexp: *regexp.MustCompile(pattern)}
}

func TestValidator_ValidatePodImages(t *testing.T) {
	tests := []struct {
		name         string
		policy       *policy.ValidatePolicy
		pod          *corev1.Pod
		wantAllowed  bool
		wantMessage  string
		wantWarnings []string
		wantError    bool
	}{
		{
			name: "deny rule matches - pod denied",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Registry: mexp("^forbidden\\.io$"),
						},
						Action:  policy.Deny,
						Message: "registry not allowed",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "forbidden.io/app:latest"},
					},
				},
			},
			wantAllowed: false,
			wantMessage: "/spec/containers/0/image: forbidden.io/app:latest (registry not allowed)",
		},
		{
			name: "warn rule matches - pod allowed with warning",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Tag: mexp("^latest$"),
						},
						Action:  policy.Warn,
						Message: "using latest tag is discouraged",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "docker.io/app:latest"},
					},
				},
			},
			wantAllowed: true,
			wantWarnings: []string{
				"/spec/containers/0/image: docker.io/app:latest (using latest tag is discouraged)",
			},
		},
		{
			name: "allow rule matches - pod allowed",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Registry: mexp("^trusted\\.io$"),
						},
						Action:  policy.Allow,
						Message: "explicitly allowed",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "trusted.io/app:v1"},
					},
				},
			},
			wantAllowed:  true,
			wantWarnings: nil,
		},
		{
			name: "no rules match - default allow",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Registry: mexp("^forbidden\\.io$"),
						},
						Action:  policy.Deny,
						Message: "denied",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "allowed.io/app:v1"},
					},
				},
			},
			wantAllowed:  true,
			wantWarnings: nil,
		},
		{
			name: "multiple containers - first deny stops validation",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Repository: mexp("^bad/.*$"),
						},
						Action:  policy.Deny,
						Message: "bad repository",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "registry.io/bad/app:v1"},
						{Image: "registry.io/good/app:v1"},
					},
				},
			},
			wantAllowed: false,
			wantMessage: "/spec/containers/0/image: registry.io/bad/app:v1 (bad repository)",
		},
		{
			name: "multiple containers - multiple warnings",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Tag: mexp("^latest$"),
						},
						Action:  policy.Warn,
						Message: "latest tag discouraged",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "app1:latest"},
						{Image: "app2:v1"},
						{Image: "app3:latest"},
					},
				},
			},
			wantAllowed: true,
			wantWarnings: []string{
				"/spec/containers/0/image: app1:latest (latest tag discouraged)",
				"/spec/containers/2/image: app3:latest (latest tag discouraged)",
			},
		},
		{
			name: "init containers are validated",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Registry: mexp("^untrusted\\.io$"),
						},
						Action:  policy.Deny,
						Message: "untrusted registry",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Image: "untrusted.io/init:v1"},
					},
				},
			},
			wantAllowed: false,
			wantMessage: "/spec/initContainers/0/image: untrusted.io/init:v1 (untrusted registry)",
		},
		{
			name: "ephemeral containers are validated",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Repository: mexp("^debug/.*$"),
						},
						Action:  policy.Warn,
						Message: "debug image",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					EphemeralContainers: []corev1.EphemeralContainer{
						{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Image: "debug/shell:latest"}},
					},
				},
			},
			wantAllowed: true,
			wantWarnings: []string{
				"/spec/ephemeralContainers/0/image: debug/shell:latest (debug image)",
			},
		},
		{
			name: "multiple rules - all checked for warnings",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Tag: mexp("^latest$"),
						},
						Action:  policy.Warn,
						Message: "latest tag",
					},
					{
						Match: policy.Match{
							Registry: mexp("^docker\\.io$"),
						},
						Action:  policy.Warn,
						Message: "public registry",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "docker.io/app:latest"},
					},
				},
			},
			wantAllowed: true,
			wantWarnings: []string{
				"/spec/containers/0/image: docker.io/app:latest (latest tag)",
				"/spec/containers/0/image: docker.io/app:latest (public registry)",
			},
		},
		{
			name: "empty pod - allowed",
			policy: &policy.ValidatePolicy{
				Rules: []policy.ValidateRule{
					{
						Match: policy.Match{
							Registry: mexp(".*"),
						},
						Action:  policy.Deny,
						Message: "deny all",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
			wantAllowed:  true,
			wantWarnings: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &Validator{Policy: tt.policy}
			result, err := v.ValidatePodImages(tt.pod)

			if tt.wantError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.wantAllowed)
			}

			if result.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", result.Message, tt.wantMessage)
			}

			if len(result.Warnings) != len(tt.wantWarnings) {
				t.Errorf("got %d warnings, want %d\nGot: %v\nWant: %v",
					len(result.Warnings), len(tt.wantWarnings), result.Warnings, tt.wantWarnings)
				return
			}

			for i, warning := range result.Warnings {
				if warning != tt.wantWarnings[i] {
					t.Errorf("Warning[%d] = %q, want %q", i, warning, tt.wantWarnings[i])
				}
			}
		})
	}
}
