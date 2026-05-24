package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
)

func (r *Runtime) ensureImage(ctx context.Context, spec *Spec) error {
	if err := r.checkAllowedImage(spec.Image); err != nil {
		return err
	}
	policy := spec.PullPolicy
	if policy == "" {
		policy = PullIfNotPresent
	}
	exists, err := r.imageExists(ctx, spec.Image)
	if err != nil {
		return err
	}
	switch policy {
	case PullNever:
		if !exists {
			return fmt.Errorf("%w: image %q not found locally", ErrImagePull, spec.Image)
		}
		return nil
	case PullIfNotPresent:
		if exists {
			return nil
		}
	case PullAlways:
	default:
		return fmt.Errorf("docker sandbox: unsupported pull policy %q", policy)
	}
	if err := r.pullImage(ctx, spec.Image, spec.RegistryAuth); err != nil {
		return fmt.Errorf("%w: %w", ErrImagePull, err)
	}
	return nil
}

func (r *Runtime) checkAllowedImage(image string) error {
	if len(r.allowedImages) == 0 {
		return nil
	}
	if _, ok := r.allowedImages[image]; ok {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrImageNotAllowed, image)
}

func (r *Runtime) imageExists(ctx context.Context, image string) (bool, error) {
	if _, err := r.client.ImageInspect(ctx, image); err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("docker inspect image %q: %w", image, err)
	}
	return true, nil
}

func (r *Runtime) pullImage(ctx context.Context, image string, auth *RegistryAuth) error {
	ctx, span := startDockerStep(ctx, "sandbox.image.pull", &Spec{Image: image}, "")
	defer span.End()
	resp, err := r.client.ImagePull(ctx, image, client.ImagePullOptions{
		RegistryAuth: encodeRegistryAuth(auth),
	})
	if err != nil {
		return fmt.Errorf("docker pull image %q: %w", image, err)
	}
	defer resp.Close()
	if err := resp.Wait(ctx); err != nil {
		_, _ = io.Copy(io.Discard, resp)
		return fmt.Errorf("docker pull image %q wait: %w", image, err)
	}
	return nil
}

func encodeRegistryAuth(auth *RegistryAuth) string {
	if auth == nil {
		return ""
	}
	payload := registry.AuthConfig{
		ServerAddress: auth.ServerAddress,
		Username:      auth.Username,
		Password:      auth.Password,
		IdentityToken: auth.IdentityToken,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}
