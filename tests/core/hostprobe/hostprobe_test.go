package tests

import (
	"context"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/hostprobe"
)

func TestHostProbeCollectsResourceSnapshot(t *testing.T) {
	probe := hostprobe.New(hostprobe.Config{
		CacheTTL:    time.Nanosecond,
		CPUInterval: time.Millisecond,
		Disks:       []hostprobe.DiskSpec{{Name: "workspace", Path: "."}},
	})

	first, err := probe.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("first snapshot: %v", err)
	}
	if first.CollectedAt.IsZero() {
		t.Fatal("collected_at must be set")
	}
	if first.CPU.LogicalCores <= 0 {
		t.Fatalf("logical cores = %d, want positive", first.CPU.LogicalCores)
	}
	if first.Memory.Total == 0 {
		t.Fatal("memory total must be positive")
	}
	if len(first.Disks) != 1 || first.Disks[0].Total == 0 {
		t.Fatalf("disks = %#v, want one usable disk", first.Disks)
	}
	if first.Network.RateReady {
		t.Fatal("first network sample must not claim a rate")
	}

	time.Sleep(2 * time.Millisecond)
	second, err := probe.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if !second.Network.RateReady {
		t.Fatal("second network sample must expose a rate")
	}
	if second.Network.SendBytesPerSecond < 0 || second.Network.RecvBytesPerSecond < 0 {
		t.Fatalf("network rate must not be negative: %#v", second.Network)
	}
}

func TestHostProbeUsesCache(t *testing.T) {
	probe := hostprobe.New(hostprobe.Config{CacheTTL: time.Minute, CPUInterval: time.Millisecond})

	first, err := probe.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("first snapshot: %v", err)
	}
	second, err := probe.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if !first.CollectedAt.Equal(second.CollectedAt) {
		t.Fatalf("cache miss: first=%s second=%s", first.CollectedAt, second.CollectedAt)
	}
}

func TestHostProbeHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := hostprobe.New(hostprobe.Config{}).Snapshot(ctx); err == nil {
		t.Fatal("expected canceled context error")
	}
}

func TestHostProbeBoundsConfiguredDiskWork(t *testing.T) {
	disks := make([]hostprobe.DiskSpec, 32)
	for index := range disks {
		disks[index] = hostprobe.DiskSpec{Name: "workspace", Path: "."}
	}
	snapshot, err := hostprobe.New(hostprobe.Config{
		CPUInterval: 10 * time.Second,
		Disks:       disks,
	}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Disks) != 16 {
		t.Fatalf("disk probes = %d, want bounded 16", len(snapshot.Disks))
	}
}
