package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/huwenlong92/sdkit/core/sandbox"
)

func TestSandboxAppliesPythonProfileAndWritesFiles(t *testing.T) {
	backend := &captureBackend{}
	runner, err := sandbox.New(sandbox.Options{Backend: backend})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	result, err := runner.Run(context.Background(), &sandbox.RunRequest{
		SubmissionID: "sub-1",
		Language:     sandbox.LanguagePython,
		Files: []sandbox.File{
			{Path: "main.py", Content: []byte(`print("ok")`)},
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
	if backend.spec == nil || backend.spec.Request.Image != "python:3.12-slim" {
		t.Fatalf("profile image = %q", backend.spec.Request.Image)
	}
	if got := backend.spec.Request.RunCmd; len(got) != 2 || got[0] != "python" || got[1] != "main.py" {
		t.Fatalf("run cmd = %#v", got)
	}
	if !backend.fileExists {
		t.Fatal("submission file should be written before backend run")
	}
	if _, err := os.Stat(backend.spec.WorkspaceDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace should be cleaned after run, stat err=%v", err)
	}
}

func TestSandboxRejectsUnsafeFilePath(t *testing.T) {
	runner, err := sandbox.New(sandbox.Options{Backend: &captureBackend{}})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	_, err = runner.Run(context.Background(), &sandbox.RunRequest{
		Image: "python:3.12-slim",
		Cmd:   []string{"python", "main.py"},
		Files: []sandbox.File{
			{Path: "../main.py", Content: []byte("x")},
		},
	})
	if !errors.Is(err, sandbox.ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestSandboxMapsBackendTimeout(t *testing.T) {
	runner, err := sandbox.New(sandbox.Options{Backend: failingBackend{err: sandbox.ErrTimeout}})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	result, err := runner.Run(context.Background(), &sandbox.RunRequest{
		Image: "python:3.12-slim",
		Cmd:   []string{"python", "main.py"},
	})
	if !errors.Is(err, sandbox.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if result == nil {
		t.Fatal("result should be returned with backend error")
	}
}

type captureBackend struct {
	spec       *sandbox.RunSpec
	fileExists bool
}

func (b *captureBackend) Run(ctx context.Context, spec *sandbox.RunSpec) (*sandbox.RunResult, error) {
	b.spec = spec
	if _, err := os.Stat(filepath.Join(spec.WorkspaceDir, "main.py")); err == nil {
		b.fileExists = true
	}
	return &sandbox.RunResult{ExitCode: 0, Stdout: []byte("ok")}, nil
}

type failingBackend struct {
	err error
}

func (b failingBackend) Run(ctx context.Context, spec *sandbox.RunSpec) (*sandbox.RunResult, error) {
	return &sandbox.RunResult{ExitCode: -1, TimedOut: errors.Is(b.err, sandbox.ErrTimeout)}, b.err
}
