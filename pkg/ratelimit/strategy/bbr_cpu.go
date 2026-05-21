package strategy

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// cpuReader 定期读取 /proc/stat 计算 CPU 使用率（0-1000）
type cpuReader struct {
	usage  int64
	stopCh chan struct{}
	once   sync.Once
}

func newCPUReader() *cpuReader {
	r := &cpuReader{stopCh: make(chan struct{})}
	go r.loop()
	return r
}

func (r *cpuReader) loop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var prevIdle, prevTotal uint64
	for {
		select {
		case <-ticker.C:
			idle, total, err := readCPUSample()
			if err != nil {
				continue
			}
			if prevTotal > 0 && total > prevTotal {
				deltaTotal := total - prevTotal
				deltaIdle := idle - prevIdle
				if deltaTotal > 0 {
					usage := (deltaTotal - deltaIdle) * 1000 / deltaTotal
					atomic.StoreInt64(&r.usage, int64(usage))
				}
			}
			prevIdle, prevTotal = idle, total
		case <-r.stopCh:
			return
		}
	}
}

func (r *cpuReader) Stop() {
	r.once.Do(func() { close(r.stopCh) })
}

func (r *cpuReader) GetUsage() int64 {
	return atomic.LoadInt64(&r.usage)
}

func readCPUSample() (idle, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0, 0, fmt.Errorf("/proc/stat empty")
	}

	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return 0, 0, fmt.Errorf("unexpected /proc/stat format")
	}

	fields := strings.Fields(line)[1:]
	if len(fields) < 4 {
		return 0, 0, fmt.Errorf("unexpected /proc/stat field count")
	}

	values := make([]uint64, len(fields))
	for i, f := range fields {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		values[i] = v
		total += v
	}

	if len(values) > 3 {
		idle = values[3]
	}
	if len(values) > 4 {
		idle += values[4]
	}

	return idle, total, nil
}
