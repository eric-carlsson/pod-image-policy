package mutate

import (
	"reflect"
	"testing"

	"github.com/eric-carlsson/pod-image-policy/pkg/config"

	corev1 "k8s.io/api/core/v1"
)

func TestRewritePodImages(t *testing.T) {
	mirror := "mirror.io"
	repo := "org/app"

	cases := []struct {
		name         string
		pod          corev1.Pod
		cfg          config.MutateConfig
		wantPatches  []PatchOp
		wantWarnings []string
		wantErr      bool
	}{
		{
			name: "replace main container",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:1.2"}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{},
				Replace: config.ImageReplace{Registry: &mirror},
			}}},
			wantPatches: []PatchOp{Replace("/spec/containers/0/image", "mirror.io/library/nginx:1.2")},
		},
		{
			name: "replace with warning message",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:1.2"}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{},
				Replace: config.ImageReplace{Registry: &mirror},
				Message: "Image rewritten to use internal mirror",
			}}},
			wantPatches:  []PatchOp{Replace("/spec/containers/0/image", "mirror.io/library/nginx:1.2")},
			wantWarnings: []string{"Image rewritten to use internal mirror: nginx:1.2 -> mirror.io/library/nginx:1.2"},
		},
		{
			name: "matched but unchanged",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "ghcr.io/org/app:1.0"}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{Repository: &repo},
				Replace: config.ImageReplace{},
			}}},
		},
		{
			name: "replace init container",
			pod:  corev1.Pod{Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Image: "busybox:1"}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{},
				Replace: config.ImageReplace{Registry: &mirror},
			}}},
			wantPatches: []PatchOp{Replace("/spec/initContainers/0/image", "mirror.io/library/busybox:1")},
		},
		{
			name: "replace ephemeral container",
			pod:  corev1.Pod{Spec: corev1.PodSpec{EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Image: "busybox:1"}}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{},
				Replace: config.ImageReplace{Registry: &mirror},
			}}},
			wantPatches: []PatchOp{Replace("/spec/ephemeralContainers/0/image", "mirror.io/library/busybox:1")},
		},
		{
			name: "no matching rule defaults allow",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "redis:7"}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{Repository: strPtr("other/repo")},
				Replace: config.ImageReplace{Tag: strPtr("stable")},
			}}},
		},
		{
			name: "all container types mutated",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers:          []corev1.Container{{Image: "nginx:1.2"}},
				InitContainers:      []corev1.Container{{Image: "busybox:1"}},
				EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Image: "alpine:3"}}},
			}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{},
				Replace: config.ImageReplace{Registry: &mirror},
			}}},
			wantPatches: []PatchOp{
				Replace("/spec/containers/0/image", "mirror.io/library/nginx:1.2"),
				Replace("/spec/initContainers/0/image", "mirror.io/library/busybox:1"),
				Replace("/spec/ephemeralContainers/0/image", "mirror.io/library/alpine:3"),
			},
		},
		{
			name: "match but no modification needed",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "mirror.io/library/nginx:1.2"}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{},
				Replace: config.ImageReplace{Registry: &mirror},
			}}},
		},
		{
			name: "first matching rule wins",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:1"}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{
				{Match: config.ImageMatch{}, Replace: config.ImageReplace{Registry: &mirror}},
				{Match: config.ImageMatch{}, Replace: config.ImageReplace{Registry: strPtr("other.io")}},
			}},
			wantPatches: []PatchOp{Replace("/spec/containers/0/image", "mirror.io/library/nginx:1")},
		},
		{
			name: "tag replaced digest preserved",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:old@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{},
				Replace: config.ImageReplace{Tag: strPtr("new")},
			}}},
			wantPatches: []PatchOp{Replace("/spec/containers/0/image", "docker.io/library/nginx:new@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		},
		{
			name: "multiple unmatched are ignored",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "redis:7"},
				{Image: "nginx:1"},
			}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{Repository: strPtr("other/repo")},
				Replace: config.ImageReplace{Tag: strPtr("stable")},
			}}},
		},
		{
			name: "mixed match and non-match",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "nginx:1"},
				{Image: "redis:7"},
			}}},
			cfg: config.MutateConfig{Rules: []config.MutateRule{{
				Match:   config.ImageMatch{Repository: strPtr("library/nginx")},
				Replace: config.ImageReplace{Registry: &mirror},
			}}},
			wantPatches: []PatchOp{Replace("/spec/containers/0/image", "mirror.io/library/nginx:1")},
		},
		{
			name: "no rules short-circuits",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "nginx:1"}}}},
			cfg:  config.MutateConfig{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutator, err := NewMutator(tc.cfg)
			if err != nil {
				if tc.wantErr {
					return
				}
				t.Fatalf("compile mutate rules: %v", err)
			}

			patches, warnings, err := mutator.RewritePodImages(&tc.pod)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("rewrite: %v", err)
			}

			if !reflect.DeepEqual(patches, tc.wantPatches) {
				t.Fatalf("patches mismatch:\n got: %#v\nwant: %#v", patches, tc.wantPatches)
			}

			if !reflect.DeepEqual(warnings, tc.wantWarnings) {
				t.Fatalf("warnings mismatch:\n got: %#v\nwant: %#v", warnings, tc.wantWarnings)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
