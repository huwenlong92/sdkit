// Package strategy BBR 自适应限流器
package strategy

import (
	"errors"
	"sync/atomic"
	"time"
)

var ErrLimitExceed = errors.New("bbr: rate limit exceeded")

// CPUGetter CPU 使用率读取接口
type CPUGetter interface {
	GetUsage() int64
	Stop()
}

// BBR 基于 CPU 负载的自适应限流器
type BBR struct {
	cpuThreshold int64
	window       time.Duration
	decay        float64

	cpu CPUGetter

	inFlight    int64
	maxInFlight int64
	prevDrop    int64

	stopCh chan struct{}
}

// BBROption BBR 配置选项
type BBROption func(*BBR)

// WithCPUThreshold 设置 CPU 阈值（0-1000），默认 800
func WithCPUThreshold(threshold int64) BBROption {
	return func(b *BBR) { b.cpuThreshold = threshold }
}

// WithWindow 设置 maxInFlight 采样间隔，默认 10s
func WithWindow(d time.Duration) BBROption {
	return func(b *BBR) { b.window = d }
}

// WithDecay 设置衰减因子（0-1），默认 0.95
func WithDecay(decay float64) BBROption {
	return func(b *BBR) { b.decay = decay }
}

// NewBBR 创建 BBR 自适应限流器
func NewBBR(opts ...BBROption) *BBR {
	b := &BBR{
		cpuThreshold: 800,
		window:       time.Second,
		decay:        0.95,
		cpu:          newCPUReader(),
		stopCh:       make(chan struct{}),
	}
	for _, opt := range opts {
		opt(b)
	}
	if b.window < 100*time.Millisecond {
		b.window = 100 * time.Millisecond
	}

	go b.tick()
	return b
}

// Stop 停止 BBR 限流器
func (l *BBR) Stop() {
	l.cpu.Stop()
	close(l.stopCh)
}

func (l *BBR) tick() {
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			inFlight := atomic.LoadInt64(&l.inFlight)
			oldMax := atomic.LoadInt64(&l.maxInFlight)

			if oldMax == 0 {
				if inFlight > 0 {
					atomic.StoreInt64(&l.maxInFlight, inFlight)
				}
			} else if inFlight > oldMax {
				atomic.StoreInt64(&l.maxInFlight, inFlight)
			} else {
				decayed := int64(float64(oldMax) * l.decay)
				if decayed < 1 {
					decayed = 1
				}
				atomic.StoreInt64(&l.maxInFlight, decayed)
			}
		case <-l.stopCh:
			return
		}
	}
}

// Allow 检查是否允许请求通过，返回 done 回调
func (l *BBR) Allow() (func(), error) {
	inFlight := atomic.AddInt64(&l.inFlight, 1)

	cpu := l.cpu.GetUsage()
	maxInFlight := atomic.LoadInt64(&l.maxInFlight)

	if maxInFlight == 0 {
		atomic.StoreInt64(&l.maxInFlight, inFlight)
		maxInFlight = inFlight
	}

	shouldDrop := false
	if cpu > l.cpuThreshold {
		if inFlight > maxInFlight {
			shouldDrop = true
		}
	} else {
		prevDrop := atomic.LoadInt64(&l.prevDrop)
		if prevDrop > 0 && time.Since(time.Unix(0, prevDrop)) < time.Second {
			if inFlight > maxInFlight {
				shouldDrop = true
			}
		}
	}

	if shouldDrop {
		atomic.AddInt64(&l.inFlight, -1)
		atomic.StoreInt64(&l.prevDrop, time.Now().UnixNano())
		return nil, ErrLimitExceed
	}

	if cpu <= l.cpuThreshold && inFlight > maxInFlight {
		atomic.StoreInt64(&l.maxInFlight, inFlight)
	}

	return func() {
		atomic.AddInt64(&l.inFlight, -1)
	}, nil
}

// SetCPU 替换 CPU 读取器（仅测试用）
func (l *BBR) SetCPU(c CPUGetter) { l.cpu = c }
