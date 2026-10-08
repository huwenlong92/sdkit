package sysprobe_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/sysprobe"
)

type hostFunc func(context.Context) (sysprobe.HostSnapshot, error)

func (f hostFunc) Snapshot(ctx context.Context) (sysprobe.HostSnapshot, error) { return f(ctx) }

type runnerFunc func(context.Context, sysprobe.DependencySpec) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, spec sysprobe.DependencySpec) ([]byte, error) {
	return f(ctx, spec)
}

func testHost(context.Context) (sysprobe.HostSnapshot, error) {
	return sysprobe.HostSnapshot{CollectedAt: time.Now()}, nil
}

func TestDependencyReadinessAndVersionRanges(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, output, minimum, maximum string
		missing, required              bool
		code                           sysprobe.DependencyCode
		compatible, ready              bool
	}{
		{name: "inclusive", output: "example v4.0.0", minimum: "4.0.0", maximum: "4.0.0", required: true, code: sysprobe.DependencyReady, compatible: true, ready: true},
		{name: "ffmpeg_prefix", output: "example n9.0.1-11-gabc", minimum: "9.0.1", maximum: "9.0.1", required: true, code: sysprobe.DependencyReady, compatible: true, ready: true},
		{name: "below", output: "3.9.9", minimum: "4.0.0", required: true, code: sysprobe.DependencyVersionOutside},
		{name: "above", output: "4.0.3", maximum: "4.0.2", required: true, code: sysprobe.DependencyVersionOutside},
		{name: "invalid_range", output: "4.0.0", minimum: "invalid", required: true, code: sysprobe.DependencyVersionOutside},
		{name: "unknown", output: "unknown build", required: true, code: sysprobe.DependencyVersionUnknown},
		{name: "missing_required", missing: true, required: true, code: sysprobe.DependencyMissing},
		{name: "missing_optional", missing: true, code: sysprobe.DependencyMissing, ready: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := binary
			if test.missing {
				path = filepath.Join(t.TempDir(), "missing")
			}
			service := sysprobe.New(sysprobe.Config{Dependencies: []sysprobe.DependencySpec{{Key: "example", BinaryPath: path, MinVersion: test.minimum, MaxVersion: test.maximum, Required: test.required}}},
				sysprobe.WithHostSampler(hostFunc(testHost)), sysprobe.WithRunner(runnerFunc(func(context.Context, sysprobe.DependencySpec) ([]byte, error) { return []byte(test.output), nil })))
			snapshot, err := service.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Dependencies) != 1 {
				t.Fatalf("dependencies = %v, want one", snapshot.Dependencies)
			}
			status := snapshot.Dependencies[0]
			if snapshot.Ready != test.ready || status.Code != test.code || status.Compatible != test.compatible || status.Installed == test.missing {
				t.Fatalf("ready=%v status=%+v, want ready=%v code=%s compatible=%v installed=%v", snapshot.Ready, status, test.ready, test.code, test.compatible, !test.missing)
			}
		})
	}
}

func TestDependencyCacheAndConfigurationIsolation(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	dependencies := []sysprobe.DependencySpec{{Key: "example", Name: "original", BinaryPath: binary, Args: []string{"--version"}, Env: []string{"EXAMPLE_MODE=probe"}}}
	service := sysprobe.New(sysprobe.Config{DependencyTTL: time.Minute, Dependencies: dependencies}, sysprobe.WithHostSampler(hostFunc(testHost)), sysprobe.WithRunner(runnerFunc(func(_ context.Context, spec sysprobe.DependencySpec) ([]byte, error) {
		calls.Add(1)
		if spec.Args[0] != "--version" || spec.Env[0] != "EXAMPLE_MODE=probe" {
			t.Errorf("spec = %+v, want copied arguments and environment", spec)
		}
		return []byte("v1.2.3"), nil
	})))
	dependencies[0].Name = "changed"
	dependencies[0].Args[0] = "changed"
	dependencies[0].Env[0] = "changed"
	first, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Dependencies) != 1 {
		t.Fatalf("dependencies=%v, want one", first.Dependencies)
	}
	first.Dependencies[0].Name = "returned mutation"
	second, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Dependencies) != 1 {
		t.Fatalf("dependencies=%v, want one", second.Dependencies)
	}
	if calls.Load() != 1 || !first.DependencyChecked.Equal(second.DependencyChecked) || second.Dependencies[0].Name != "original" {
		t.Fatalf("calls=%d checked=%s/%s name=%s, want cached original result", calls.Load(), first.DependencyChecked, second.DependencyChecked, second.Dependencies[0].Name)
	}
}

func TestCanceledDependencyCheckDoesNotPopulateCache(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	service := sysprobe.New(sysprobe.Config{DependencyTTL: time.Minute, Dependencies: []sysprobe.DependencySpec{{BinaryPath: binary, Required: true}}}, sysprobe.WithHostSampler(hostFunc(testHost)), sysprobe.WithRunner(runnerFunc(func(ctx context.Context, _ sysprobe.DependencySpec) ([]byte, error) {
		if calls.Add(1) == 1 {
			cancel()
			return nil, ctx.Err()
		}
		return []byte("v1.2.3"), nil
	})))
	if _, err := service.Snapshot(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want canceled", err)
	}
	snapshot, err := service.Snapshot(context.Background())
	if err != nil || !snapshot.Ready || calls.Load() != 2 {
		t.Fatalf("snapshot=%+v error=%v calls=%d, want fresh ready snapshot", snapshot, err, calls.Load())
	}
}

func TestDependencyTimeoutAndProbeDirectory(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "probe")
	service := sysprobe.New(sysprobe.Config{CheckTimeout: time.Millisecond, Dependencies: []sysprobe.DependencySpec{{BinaryPath: binary, PrepareDir: directory, Env: []string{"EXAMPLE_HOME=" + directory}, Required: true}}}, sysprobe.WithHostSampler(hostFunc(testHost)), sysprobe.WithRunner(runnerFunc(func(ctx context.Context, spec sysprobe.DependencySpec) ([]byte, error) {
		if _, err := os.Stat(directory); err != nil {
			t.Errorf("directory=%s error=%v", directory, err)
		}
		if len(spec.Env) != 1 || spec.Env[0] != "EXAMPLE_HOME="+directory {
			t.Errorf("environment=%v, want caller-provided environment only", spec.Env)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})))
	snapshot, err := service.Snapshot(context.Background())
	if err != nil || snapshot.Ready || len(snapshot.Dependencies) != 1 || snapshot.Dependencies[0].Code != sysprobe.DependencyCheckFailed {
		t.Fatalf("snapshot=%+v error=%v, want timeout failure", snapshot, err)
	}
}

func TestHostFailuresAndServiceClose(t *testing.T) {
	cause := errors.New("collector failed")
	tests := []struct {
		name      string
		host      hostFunc
		wantError bool
		wantIssue bool
	}{
		{name: "error", host: func(context.Context) (sysprobe.HostSnapshot, error) { return sysprobe.HostSnapshot{}, cause }, wantError: true},
		{name: "panic", host: func(context.Context) (sysprobe.HostSnapshot, error) { panic("collector failure") }, wantIssue: true},
		{name: "normal", host: testHost},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := sysprobe.New(sysprobe.Config{NetworkCapacity: sysprobe.NetworkCapacityConfig{ReceiveLimitMbps: -1, SendLimitMbps: 3}}, sysprobe.WithHostSampler(test.host))
			snapshot, err := service.Snapshot(context.Background())
			if test.wantError {
				if !errors.Is(err, cause) {
					t.Fatalf("error=%v, want %v", err, cause)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if (len(snapshot.Host.Issues) > 0) != test.wantIssue || snapshot.NetworkCapacity.ReceiveLimitMbps != 0 || snapshot.NetworkCapacity.SendLimitMbps != 3 {
					t.Fatalf("snapshot=%+v, want issue=%v and normalized capacity", snapshot, test.wantIssue)
				}
			}
			if err := service.Close(); err != nil {
				t.Fatal(err)
			}
			if err := service.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Snapshot(context.Background()); !errors.Is(err, sysprobe.ErrUnavailable) {
				t.Fatalf("closed error=%v, want unavailable", err)
			}
		})
	}
}

func TestRealCommandRunner(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("shell is unavailable")
	}
	service := sysprobe.New(sysprobe.Config{Dependencies: []sysprobe.DependencySpec{{BinaryPath: "sh", Args: []string{"-c", "printf 'example version v1.2.3'"}, Required: true}}}, sysprobe.WithHostSampler(hostFunc(testHost)))
	snapshot, err := service.Snapshot(context.Background())
	if err != nil || !snapshot.Ready || len(snapshot.Dependencies) != 1 || snapshot.Dependencies[0].Version != "1.2.3" {
		t.Fatalf("snapshot=%+v error=%v, want real executable version 1.2.3", snapshot, err)
	}
}

func TestWaitingSnapshotHonorsCancellationAndClose(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	service := sysprobe.New(sysprobe.Config{Dependencies: []sysprobe.DependencySpec{{BinaryPath: binary, Required: true}}}, sysprobe.WithHostSampler(hostFunc(testHost)), sysprobe.WithRunner(runnerFunc(func(ctx context.Context, _ sysprobe.DependencySpec) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})))
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Error(err)
		}
	})
	firstDone := make(chan error, 1)
	go func() { _, err := service.Snapshot(context.Background()); firstDone <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dependency check did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	waiting := make(chan error, 1)
	go func() { _, err := service.Snapshot(ctx); waiting <- err }()
	select {
	case err := <-waiting:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiting error=%v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled snapshot remained blocked behind dependency check")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("active snapshot error=%v, want canceled on close", err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not cancel active dependency check")
	}
}
