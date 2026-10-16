package admission

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestCollectImageSlots(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Image: "nginx:1.0"},
				{Image: "redis:latest"},
			},
			InitContainers: []corev1.Container{
				{Image: "busybox:1.2"},
			},
			EphemeralContainers: []corev1.EphemeralContainer{
				{Image: "debug:v1"},
			},
			Volumes: []corev1.Volume{
				{
					Image: &corev1.ImageVolumeSource{
						Reference: "ghcr.io/foo/bar:v1.0.0",
					},
				},
				{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
	}

	slots := CollectImageSlots(pod)

	expected := []ImageSlot{
		{Image: "nginx:1.0", Path: "/spec/containers/0/image"},
		{Image: "redis:latest", Path: "/spec/containers/1/image"},
		{Image: "busybox:1.2", Path: "/spec/initContainers/0/image"},
		{Image: "debug:v1", Path: "/spec/ephemeralContainers/0/image"},
		{Image: "ghcr.io/foo/bar:v1.0.0", Path: "/spec/volumes/0/image/reference"},
	}

	if len(slots) != len(expected) {
		t.Fatalf("expected %d slots, got %d", len(expected), len(slots))
	}

	for i, exp := range expected {
		if slots[i] != exp {
			t.Errorf("slot %d: expected %+v, got %+v", i, exp, slots[i])
		}
	}
}
