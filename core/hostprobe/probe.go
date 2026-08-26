package hostprobe

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

const (
	defaultCacheTTL    = 2 * time.Second
	defaultCPUInterval = 200 * time.Millisecond
	maxCPUInterval     = time.Second
	maxDiskSpecs       = 16
)

type Probe struct {
	config Config

	mu          sync.Mutex
	cached      Snapshot
	cachedAt    time.Time
	lastNetwork networkSample
}

type networkSample struct {
	at       time.Time
	bytesOut uint64
	bytesIn  uint64
}

func New(config Config) *Probe {
	return &Probe{config: normalizeConfig(config)}
}

func (p *Probe) Snapshot(ctx context.Context) (Snapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if p == nil {
		p = New(Config{})
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if !p.cachedAt.IsZero() && now.Sub(p.cachedAt) < p.config.CacheTTL {
		return cloneSnapshot(p.cached), nil
	}

	snapshot := p.collect(ctx)
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	p.cached = snapshot
	p.cachedAt = snapshot.CollectedAt
	return cloneSnapshot(snapshot), nil
}

func (p *Probe) collect(ctx context.Context) Snapshot {
	snapshot := Snapshot{
		Disks:  make([]Disk, 0, len(p.config.Disks)),
		Issues: make([]Issue, 0),
	}

	if info, err := host.InfoWithContext(ctx); err != nil {
		snapshot.addIssue("host", "", err)
	} else if info != nil {
		snapshot.Host = Host{
			Hostname:        info.Hostname,
			OS:              info.OS,
			Platform:        info.Platform,
			PlatformVersion: info.PlatformVersion,
			KernelVersion:   info.KernelVersion,
			Architecture:    runtime.GOARCH,
			UptimeSeconds:   info.Uptime,
			BootTime:        info.BootTime,
		}
	}

	if cores, err := cpu.CountsWithContext(ctx, true); err != nil {
		snapshot.addIssue("cpu", "", err)
	} else {
		snapshot.CPU.LogicalCores = cores
	}
	if percents, err := cpu.PercentWithContext(ctx, p.config.CPUInterval, false); err != nil {
		snapshot.addIssue("cpu", "", err)
	} else if len(percents) > 0 {
		snapshot.CPU.UsedPercent = clampPercent(percents[0])
	}

	if stat, err := mem.VirtualMemoryWithContext(ctx); err != nil {
		snapshot.addIssue("memory", "", err)
	} else if stat != nil {
		snapshot.Memory = Memory{
			Total:       stat.Total,
			Used:        stat.Used,
			Available:   stat.Available,
			UsedPercent: clampPercent(stat.UsedPercent),
		}
	}

	for _, spec := range p.config.Disks {
		stat, err := disk.UsageWithContext(ctx, spec.Path)
		if err != nil {
			snapshot.addIssue("disk", spec.Path, err)
			continue
		}
		if stat == nil {
			continue
		}
		snapshot.Disks = append(snapshot.Disks, Disk{
			Name:        spec.Name,
			Path:        spec.Path,
			Filesystem:  stat.Fstype,
			Total:       stat.Total,
			Used:        stat.Used,
			Free:        stat.Free,
			UsedPercent: clampPercent(stat.UsedPercent),
		})
	}

	if counters, err := gnet.IOCountersWithContext(ctx, false); err != nil {
		snapshot.addIssue("network", "", err)
	} else if len(counters) > 0 {
		now := time.Now()
		counter := counters[0]
		snapshot.Network = Network{
			BytesSent:       counter.BytesSent,
			BytesReceived:   counter.BytesRecv,
			PacketsSent:     counter.PacketsSent,
			PacketsReceived: counter.PacketsRecv,
		}
		if !p.lastNetwork.at.IsZero() {
			seconds := now.Sub(p.lastNetwork.at).Seconds()
			if seconds > 0 && counter.BytesSent >= p.lastNetwork.bytesOut && counter.BytesRecv >= p.lastNetwork.bytesIn {
				snapshot.Network.SendBytesPerSecond = float64(counter.BytesSent-p.lastNetwork.bytesOut) / seconds
				snapshot.Network.RecvBytesPerSecond = float64(counter.BytesRecv-p.lastNetwork.bytesIn) / seconds
				snapshot.Network.SampleWindow = seconds
				snapshot.Network.RateReady = true
			}
		}
		p.lastNetwork = networkSample{at: now, bytesOut: counter.BytesSent, bytesIn: counter.BytesRecv}
	}

	snapshot.CollectedAt = time.Now()
	return snapshot
}

func normalizeConfig(config Config) Config {
	if config.CacheTTL <= 0 {
		config.CacheTTL = defaultCacheTTL
	}
	if config.CPUInterval <= 0 || config.CPUInterval > maxCPUInterval {
		config.CPUInterval = defaultCPUInterval
	}
	disks := make([]DiskSpec, 0, len(config.Disks))
	for _, spec := range config.Disks {
		if len(disks) >= maxDiskSpecs {
			break
		}
		spec.Path = strings.TrimSpace(spec.Path)
		if spec.Path == "" {
			continue
		}
		spec.Name = strings.TrimSpace(spec.Name)
		if spec.Name == "" {
			spec.Name = spec.Path
		}
		disks = append(disks, spec)
	}
	if len(disks) == 0 {
		disks = append(disks, DiskSpec{Name: "workdir", Path: "."})
	}
	config.Disks = disks
	return config
}

func (s *Snapshot) addIssue(component string, path string, err error) {
	if err == nil {
		return
	}
	s.Issues = append(s.Issues, Issue{Component: component, Path: path, Message: err.Error()})
}

func clampPercent(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 100:
		return 100
	default:
		return value
	}
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Disks = append([]Disk(nil), snapshot.Disks...)
	snapshot.Issues = append([]Issue(nil), snapshot.Issues...)
	return snapshot
}
