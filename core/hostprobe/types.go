// Package hostprobe collects lightweight host resource snapshots for
// operational dashboards and health inspection.
package hostprobe

import "time"

type DiskSpec struct {
	Name string `json:"name" mapstructure:"name" yaml:"name"`
	Path string `json:"path" mapstructure:"path" yaml:"path"`
}

type Config struct {
	CacheTTL    time.Duration `json:"-" mapstructure:"cache_ttl" yaml:"cache_ttl"`
	CPUInterval time.Duration `json:"-" mapstructure:"cpu_interval" yaml:"cpu_interval"`
	Disks       []DiskSpec    `json:"-" mapstructure:"disks" yaml:"disks"`
}

type Snapshot struct {
	CollectedAt time.Time `json:"collected_at"`
	Host        Host      `json:"host"`
	CPU         CPU       `json:"cpu"`
	Memory      Memory    `json:"memory"`
	Disks       []Disk    `json:"disks"`
	Network     Network   `json:"network"`
	Issues      []Issue   `json:"issues"`
}

type Host struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	KernelVersion   string `json:"kernel_version"`
	Architecture    string `json:"architecture"`
	UptimeSeconds   uint64 `json:"uptime_seconds"`
	BootTime        uint64 `json:"boot_time"`
}

type CPU struct {
	LogicalCores int     `json:"logical_cores"`
	UsedPercent  float64 `json:"used_percent"`
}

type Memory struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"used_percent"`
}

type Disk struct {
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	Filesystem  string  `json:"filesystem"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

type Network struct {
	BytesSent          uint64  `json:"bytes_sent"`
	BytesReceived      uint64  `json:"bytes_received"`
	PacketsSent        uint64  `json:"packets_sent"`
	PacketsReceived    uint64  `json:"packets_received"`
	SendBytesPerSecond float64 `json:"send_bytes_per_second"`
	RecvBytesPerSecond float64 `json:"recv_bytes_per_second"`
	SampleWindow       float64 `json:"sample_window_seconds"`
	RateReady          bool    `json:"rate_ready"`
}

type Issue struct {
	Component string `json:"component"`
	Path      string `json:"path,omitempty"`
	Message   string `json:"message"`
}
