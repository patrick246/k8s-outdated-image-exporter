package docker

import (
	"context"
	"log/slog"
	"strings"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"github.com/patrick246/k8s-outdated-image-exporter/pkg/clients"
)

type ContainerClient struct {
	client *client.Client
	logger *slog.Logger
}

func NewDockerClient(logger *slog.Logger) (*ContainerClient, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &ContainerClient{
		client: dockerClient,
		logger: logger,
	}, nil
}

func (c *ContainerClient) Listener(ctx context.Context) (<-chan clients.ContainerImage, error) {
	containerImageChannel := make(chan clients.ContainerImage)

	containers, err := c.client.ContainerList(ctx, dockercontainer.ListOptions{
		All: false,
	})
	if err != nil {
		return nil, err
	}

	go func() {
		for _, container := range containers {
			c.logger.Debug("container info", "container", container)

			c.handleCreated(ctx, containerImageChannel, container.ID, firstNameOrID(container), container.Image)
		}

		messages, errorChan := c.client.Events(ctx, types.EventsOptions{})

		for {
			select {
			case message := <-messages:
				if message.Type != "container" {
					continue
				}

				switch message.Action {
				case "create":
					c.handleCreated(ctx, containerImageChannel, message.Actor.ID, message.Actor.Attributes["name"], message.Actor.Attributes["image"])
				case "die":
					containerImageChannel <- clients.ContainerImage{
						Action:      clients.ContainerImageRemoved,
						Name:        message.Actor.Attributes["name"],
						Metadata:    nil,
						Labels:      nil,
						Annotations: nil,
						Image:       message.Actor.Attributes["image"],
					}
				}

				c.logger.Debug("docker event", "event", message)
			case err := <-errorChan:
				c.logger.Error("error reading docker events", "error", err)

				break
			}
		}
	}()

	return containerImageChannel, nil
}

func (c *ContainerClient) handleCreated(ctx context.Context, containerImageChannel chan<- clients.ContainerImage, containerID, name, image string) {
	labels, err := c.getContainerLabels(ctx, containerID)
	if err != nil {
		c.logger.Warn("error getting container labels", "error", err, "id", containerID)
	}

	containerImageChannel <- clients.ContainerImage{
		Action:      clients.ContainerImageAdded,
		Name:        name,
		Metadata:    nil,
		Labels:      labels,
		Annotations: labels,
		Image:       image,
	}
}

func (c *ContainerClient) getContainerLabels(ctx context.Context, containerID string) (map[string]string, error) {
	containerDetails, err := c.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	return containerDetails.Config.Labels, nil
}

func firstNameOrID(container types.Container) string {
	if len(container.Names) > 0 {
		return strings.TrimLeft(container.Names[0], "/")
	}

	return container.ID
}
