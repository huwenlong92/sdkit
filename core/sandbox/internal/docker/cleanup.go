package docker

import (
	"context"
	"fmt"

	"github.com/moby/moby/client"
)

func (r *Runtime) remove(ctx context.Context, containerID string) error {
	if containerID == "" {
		return nil
	}
	ctx, span := startDockerStep(ctx, "sandbox.cleanup", nil, "")
	defer span.End()
	if _, err := r.client.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("docker remove container %s: %w", containerID, err)
	}
	return nil
}
