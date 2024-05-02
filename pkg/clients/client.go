package clients

type Action int

const (
	ContainerImageAdded Action = iota
	ContainerImageRemoved
)

func (a Action) String() string {
	switch a {
	case ContainerImageAdded:
		return "ContainerImageAdded"
	case ContainerImageRemoved:
		return "ContainerImageRemoved"
	}

	return "Unknown"
}

type ContainerImage struct {
	Action Action

	// Name identifying the container
	Name string

	// Outdated image exporter metadata, like Pod, Namespace, Pull Secrets
	Metadata map[string]interface{}

	// Source labels, e.g. K8s labels, Docker labels
	Labels map[string]string

	// Source annotations, e.g. K8s annotations
	Annotations map[string]string

	// Image reference, including registry, name and tag
	Image string
}
