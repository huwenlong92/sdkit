package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/huwenlong92/sdkit/core/sandbox/security"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type createdContainer struct {
	ID string
}

func (r *Runtime) create(ctx context.Context, spec *Spec, phase string, cmd []string, stdin []byte) (*createdContainer, error) {
	ctx, span := startDockerStep(ctx, "sandbox.create", spec, phase)
	defer span.End()

	sec := security.Default()
	sec.NetworkDisabled = spec.NetworkDisabled
	sec.ReadonlyRootfs = spec.ReadonlyRootfs

	pidsLimit := spec.PidsLimit
	securityOpt := []string{"no-new-privileges:true"}
	if sec.SeccompProfile != "" {
		securityOpt = append(securityOpt, "seccomp="+sec.SeccompProfile)
	}
	hostConfig := &container.HostConfig{
		AutoRemove:     false,
		NetworkMode:    container.NetworkMode("none"),
		ReadonlyRootfs: spec.ReadonlyRootfs,
		CapDrop:        sec.CapDrop,
		SecurityOpt:    securityOpt,
		Tmpfs:          sec.Tmpfs,
		IpcMode:        container.IpcMode("private"),
		Resources: container.Resources{
			NanoCPUs:   spec.CPUNano,
			Memory:     spec.MemoryBytes,
			MemorySwap: spec.MemoryBytes,
			PidsLimit:  &pidsLimit,
			Ulimits:    toDockerUlimits(spec.Ulimits),
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: spec.WorkspaceDir,
				Target: spec.WorkingDir,
			},
		},
	}
	if !spec.NetworkDisabled {
		hostConfig.NetworkMode = container.NetworkMode("bridge")
	}

	stopTimeout := int((2 * time.Second).Seconds())
	cfg := &container.Config{
		Image:           spec.Image,
		Cmd:             cmd,
		Env:             spec.Env,
		WorkingDir:      spec.WorkingDir,
		User:            spec.User,
		AttachStdout:    true,
		AttachStderr:    true,
		AttachStdin:     len(stdin) > 0,
		OpenStdin:       len(stdin) > 0,
		StdinOnce:       len(stdin) > 0,
		NetworkDisabled: spec.NetworkDisabled,
		StopTimeout:     &stopTimeout,
		Labels: map[string]string{
			"sdkit.sandbox":       "true",
			"sdkit.submission_id": spec.SubmissionID,
			"sdkit.phase":         phase,
		},
	}
	res, err := r.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     cfg,
		HostConfig: hostConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("docker create container: %w", err)
	}
	return &createdContainer{ID: res.ID}, nil
}

func (r *Runtime) start(ctx context.Context, containerID string) error {
	ctx, span := startDockerStep(ctx, "sandbox.start", nil, "")
	defer span.End()
	if _, err := r.client.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("docker start container %s: %w", containerID, err)
	}
	return nil
}

func (r *Runtime) wait(ctx context.Context, containerID string) (*container.WaitResponse, error) {
	ctx, span := startDockerStep(ctx, "sandbox.wait", nil, "")
	defer span.End()
	wait := r.client.ContainerWait(ctx, containerID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})
	select {
	case res := <-wait.Result:
		if res.Error != nil {
			return &res, fmt.Errorf("docker wait container %s: %s", containerID, res.Error.Message)
		}
		return &res, nil
	case err := <-wait.Error:
		return nil, fmt.Errorf("docker wait container %s: %w", containerID, err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *Runtime) kill(ctx context.Context, containerID string) error {
	if _, err := r.client.ContainerKill(ctx, containerID, client.ContainerKillOptions{}); err != nil {
		return fmt.Errorf("docker kill container %s: %w", containerID, err)
	}
	return nil
}

func (r *Runtime) attachStdin(ctx context.Context, containerID string, stdin []byte) error {
	attached, err := r.client.ContainerAttach(ctx, containerID, client.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return fmt.Errorf("docker attach stdin %s: %w", containerID, err)
	}
	go func() {
		defer attached.Close()
		defer attached.CloseWrite()
		_, _ = io.Copy(attached.Conn, bytes.NewReader(stdin))
	}()
	return nil
}

func toDockerUlimits(in []Ulimit) []*container.Ulimit {
	out := make([]*container.Ulimit, 0, len(in))
	for _, item := range in {
		if item.Name == "" {
			continue
		}
		out = append(out, &container.Ulimit{Name: item.Name, Soft: item.Soft, Hard: item.Hard})
	}
	return out
}
