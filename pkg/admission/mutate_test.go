package admission

import (
	"testing"

	"github.com/eric-carlsson/pod-image-policy/pkg/policy"
	corev1 "k8s.io/api/core/v1"
)

func TestMutator_MutatePodImages(t *testing.T) {
	tests := []struct {
		name         string
		policy       *policy.MutatePolicy
		pod          *corev1.Pod
		wantPatches  []Patch
		wantWarnings []string
		wantError    bool
	}{
		{
			name: "simple registry replacement",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Registry: mexp("^old\\.io$"),
						},
						Replace: policy.Replace{
							Registry: new("new.io"),
						},
						Message: "migrated to new registry",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "old.io/app:v1"},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/containers/0/image",
					Value: "new.io/app:v1",
				},
			},
			wantWarnings: []string{
				"/spec/containers/0/image: old.io/app:v1 -> new.io/app:v1 (migrated to new registry)",
			},
		},
		{
			name: "repository replacement with captures",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Repository: mexp("^team/(.*)$"),
						},
						Replace: policy.Replace{
							Repository: new("project/${1}"),
						},
						Message: "repository restructure",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "registry.io/team/app:v1"},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/containers/0/image",
					Value: "registry.io/project/app:v1",
				},
			},
			wantWarnings: []string{
				"/spec/containers/0/image: registry.io/team/app:v1 -> registry.io/project/app:v1 (repository restructure)",
			},
		},
		{
			name: "tag replacement with captures",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Tag: mexp("^v(\\d+)\\.(\\d+)$"),
						},
						Replace: policy.Replace{
							Tag: new("release-${1}.${2}"),
						},
						Message: "tag format updated",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "app:v1.2"},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/containers/0/image",
					Value: "docker.io/library/app:release-1.2",
				},
			},
			wantWarnings: []string{
				"/spec/containers/0/image: app:v1.2 -> docker.io/library/app:release-1.2 (tag format updated)",
			},
		},
		{
			name: "multiple fields replaced",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Registry:   mexp("^old\\.io$"),
							Repository: mexp("^(.*)$"),
						},
						Replace: policy.Replace{
							Registry:   new("new.io"),
							Repository: new("migrated/${1}"),
						},
						Message: "full migration",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "old.io/app:v1"},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/containers/0/image",
					Value: "new.io/migrated/app:v1",
				},
			},
			wantWarnings: []string{
				"/spec/containers/0/image: old.io/app:v1 -> new.io/migrated/app:v1 (full migration)",
			},
		},
		{
			name: "no match - no patches",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Registry: mexp("^other\\.io$"),
						},
						Replace: policy.Replace{
							Registry: new("new.io"),
						},
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "current.io/app:v1"},
					},
				},
			},
			wantPatches:  nil,
			wantWarnings: nil,
		},
		{
			name: "multiple containers - some mutated",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Registry: mexp("^old\\.io$"),
						},
						Replace: policy.Replace{
							Registry: new("new.io"),
						},
						Message: "migrated",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "old.io/app1:v1"},
						{Image: "other.io/app2:v1"},
						{Image: "old.io/app3:v2"},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/containers/0/image",
					Value: "new.io/app1:v1",
				},
				{
					Op:    "replace",
					Path:  "/spec/containers/2/image",
					Value: "new.io/app3:v2",
				},
			},
			wantWarnings: []string{
				"/spec/containers/0/image: old.io/app1:v1 -> new.io/app1:v1 (migrated)",
				"/spec/containers/2/image: old.io/app3:v2 -> new.io/app3:v2 (migrated)",
			},
		},
		{
			name: "init containers mutated",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Tag: mexp("^latest$"),
						},
						Replace: policy.Replace{
							Tag: new("stable"),
						},
						Message: "pinned tag",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Image: "init:latest"},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/initContainers/0/image",
					Value: "docker.io/library/init:stable",
				},
			},
			wantWarnings: []string{
				"/spec/initContainers/0/image: init:latest -> docker.io/library/init:stable (pinned tag)",
			},
		},
		{
			name: "ephemeral containers mutated",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Registry: mexp("^public\\.io$"),
						},
						Replace: policy.Replace{
							Registry: new("internal.io"),
						},
						Message: "use internal registry",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					EphemeralContainers: []corev1.EphemeralContainer{
						{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Image: "public.io/debug:latest"}},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/ephemeralContainers/0/image",
					Value: "internal.io/debug:latest",
				},
			},
			wantWarnings: []string{
				"/spec/ephemeralContainers/0/image: public.io/debug:latest -> internal.io/debug:latest (use internal registry)",
			},
		},
		{
			name: "first matching rule wins",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Registry: mexp("^old\\.io$"),
						},
						Replace: policy.Replace{
							Registry: new("new.io"),
						},
						Message: "rule 1",
					},
					{
						Match: policy.Match{
							Registry: mexp("^old\\.io$"),
						},
						Replace: policy.Replace{
							Registry: new("other.io"),
						},
						Message: "rule 2",
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "old.io/app:v1"},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/containers/0/image",
					Value: "new.io/app:v1",
				},
			},
			wantWarnings: []string{
				"/spec/containers/0/image: old.io/app:v1 -> new.io/app:v1 (rule 1)",
			},
		},
		{
			name: "no message - no warning",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Registry: mexp("^old\\.io$"),
						},
						Replace: policy.Replace{
							Registry: new("new.io"),
						},
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "old.io/app:v1"},
					},
				},
			},
			wantPatches: []Patch{
				{
					Op:    "replace",
					Path:  "/spec/containers/0/image",
					Value: "new.io/app:v1",
				},
			},
			wantWarnings: nil,
		},
		{
			name: "missing capture group - error",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Tag: mexp("^v(\\d+)$"),
						},
						Replace: policy.Replace{
							Tag: new("v${2}"),
						},
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "app:v1"},
					},
				},
			},
			wantError: true,
		},
		{
			name: "empty pod - no patches",
			policy: &policy.MutatePolicy{
				Rules: []policy.MutateRule{
					{
						Match: policy.Match{
							Registry: mexp(".*"),
						},
						Replace: policy.Replace{
							Registry: new("new.io"),
						},
					},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
			wantPatches:  nil,
			wantWarnings: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Mutator{Policy: tt.policy}
			result, err := m.MutatePodImages(tt.pod)

			if tt.wantError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Patches) != len(tt.wantPatches) {
				t.Errorf("got %d patches, want %d\nGot: %+v\nWant: %+v",
					len(result.Patches), len(tt.wantPatches), result.Patches, tt.wantPatches)
				return
			}

			for i, patch := range result.Patches {
				want := tt.wantPatches[i]
				if patch.Op != want.Op || patch.Path != want.Path || patch.Value != want.Value {
					t.Errorf("Patch[%d] = %+v, want %+v", i, patch, want)
				}
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
