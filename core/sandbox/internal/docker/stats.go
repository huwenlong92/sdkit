package docker

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type statsSnapshot struct {
	MemoryUsed uint64
	CPUUsed    float64
}

func (r *Runtime) collectStats(ctx context.Context, containerID string) <-chan statsSnapshot {
	out := make(chan statsSnapshot, 1)
	go func() {
		defer close(out)
		out <- r.readStats(ctx, containerID)
	}()
	return out
}

func (r *Runtime) readStats(ctx context.Context, containerID string) statsSnapshot {
	resp, err := r.client.ContainerStats(ctx, containerID, client.ContainerStatsOptions{Stream: true})
	if err != nil {
		return statsSnapshot{}
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var peak uint64
	var cpu uint64
	for {
		var stat container.StatsResponse
		if err := decoder.Decode(&stat); err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return statsSnapshot{MemoryUsed: peak, CPUUsed: float64(cpu) / 1e9}
			}
			return statsSnapshot{MemoryUsed: peak, CPUUsed: float64(cpu) / 1e9}
		}
		if stat.MemoryStats.Usage > peak {
			peak = stat.MemoryStats.Usage
		}
		if stat.MemoryStats.MaxUsage > peak {
			peak = stat.MemoryStats.MaxUsage
		}
		if stat.CPUStats.CPUUsage.TotalUsage > cpu {
			cpu = stat.CPUStats.CPUUsage.TotalUsage
		}
		select {
		case <-ctx.Done():
			return statsSnapshot{MemoryUsed: peak, CPUUsed: float64(cpu) / 1e9}
		default:
		}
	}
}
