package admission

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// ImageSlot represents a container image location in a pod with its JSON path.
type ImageSlot struct {
	Image string
	Path  string
}

// CollectImageSlots returns all container images in a pod with their JSON patch paths.
func CollectImageSlots(pod *corev1.Pod) []ImageSlot {
	var slots []ImageSlot

	for i, c := range pod.Spec.Containers {
		slots = append(slots, ImageSlot{
			Image: c.Image,
			Path:  fmt.Sprintf("/spec/containers/%d/image", i),
		})
	}
	for i, c := range pod.Spec.InitContainers {
		slots = append(slots, ImageSlot{
			Image: c.Image,
			Path:  fmt.Sprintf("/spec/initContainers/%d/image", i),
		})
	}
	for i, c := range pod.Spec.EphemeralContainers {
		slots = append(slots, ImageSlot{
			Image: c.Image,
			Path:  fmt.Sprintf("/spec/ephemeralContainers/%d/image", i),
		})
	}
	for i, c := range pod.Spec.Volumes {
		if c.Image != nil {
			slots = append(slots, ImageSlot{
				Image: c.Image.Reference,
				Path:  fmt.Sprintf("/spec/volumes/%d/image/reference", i),
			})
		}
	}

	return slots
}
