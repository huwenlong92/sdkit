package sysprobe_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/sysprobe"
)

func TestHostProbeCollectsResourceSnapshot(t *testing.T) {
	probe := sysprobe.NewHost(sysprobe.HostConfig{
		CacheTTL:    time.Nanosecond,
		CPUInterval: time.Millisecond,
		Disks:       []sysprobe.DiskSpec{{Name: "workspace", Path: "."}},
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
	probe := sysprobe.NewHost(sysprobe.HostConfig{CacheTTL: time.Minute, CPUInterval: time.Millisecond})

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

func TestHostProbeUsesConfiguredNetworkProcRootAndInterfaces(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("procfs network source is Linux-specific")
	}

	procRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "net"), 0o700); err != nil {
		t.Fatalf("create proc net directory: %v", err)
	}
	writeNetworkCounters := func(ethRecv uint64, ethSent uint64, dockerRecv uint64, dockerSent uint64) {
		t.Helper()
		contents := fmt.Sprintf(`Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: %d 10 0 0 0 0 0 0 %d 20 0 0 0 0 0 0
docker0: %d 30 0 0 0 0 0 0 %d 40 0 0 0 0 0 0
`, ethRecv, ethSent, dockerRecv, dockerSent)
		if err := os.WriteFile(filepath.Join(procRoot, "net", "dev"), []byte(contents), 0o600); err != nil {
			t.Fatalf("write network counters: %v", err)
		}
	}
	writeNetworkCounters(1000, 2000, 100000, 200000)

	probe := sysprobe.NewHost(sysprobe.HostConfig{
		CacheTTL:    time.Nanosecond,
		CPUInterval: time.Millisecond,
		Network: sysprobe.NetworkSpec{
			ProcRoot:   procRoot,
			Interfaces: []string{"eth0"},
		},
	})
	first, err := probe.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("first snapshot: %v", err)
	}
	if first.Network.BytesReceived != 1000 || first.Network.BytesSent != 2000 {
		t.Fatalf("first network = %#v, want eth0 counters only", first.Network)
	}

	writeNetworkCounters(1600, 2800, 300000, 500000)
	time.Sleep(2 * time.Millisecond)
	second, err := probe.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if second.Network.BytesReceived != 1600 || second.Network.BytesSent != 2800 {
		t.Fatalf("second network = %#v, want eth0 counters only", second.Network)
	}
	if !second.Network.RateReady || second.Network.RecvBytesPerSecond <= 0 || second.Network.SendBytesPerSecond <= 0 {
		t.Fatalf("second network rate = %#v, want positive ready rate", second.Network)
	}
}

func TestHostProbeHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := sysprobe.NewHost(sysprobe.HostConfig{}).Snapshot(ctx); err == nil {
		t.Fatal("expected canceled context error")
	}
}

func TestHostProbeBoundsConfiguredDiskWork(t *testing.T) {
	disks := make([]sysprobe.DiskSpec, 32)
	for index := range disks {
		disks[index] = sysprobe.DiskSpec{Name: "workspace", Path: "."}
	}
	snapshot, err := sysprobe.NewHost(sysprobe.HostConfig{
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
