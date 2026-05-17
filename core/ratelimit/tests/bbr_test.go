package tests

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/ratelimit/strategy"
)

// mockCPU 模拟 CPU 使用率
type mockCPU struct {
	usage int64
}

func (m *mockCPU) GetUsage() int64 { return atomic.LoadInt64(&m.usage) }
func (m *mockCPU) Stop()           {}
func (m *mockCPU) set(u int64)     { atomic.StoreInt64(&m.usage, u) }

// withMockCPU 注入 mock CPU 读取器
func withMockCPU(m *mockCPU) strategy.BBROption {
	return func(b *strategy.BBR) { b.SetCPU(m) }
}

func TestBBR_LowCPU_AllPass(t *testing.T) {
	mcpu := &mockCPU{}
	mcpu.set(100)

	bb := strategy.NewBBR(
		withMockCPU(mcpu),
		strategy.WithWindow(200*time.Millisecond),
	)
	defer bb.Stop()

	for i := 0; i < 1000; i++ {
		done, err := bb.Allow()
		if err != nil {
			t.Fatalf("低 CPU 时请求 %d 应该通过, got: %v", i, err)
		}
		time.Sleep(time.Microsecond)
		done()
	}
}

func TestBBR_HighCPU_Throttle(t *testing.T) {
	mcpu := &mockCPU{}
	mcpu.set(100)

	bb := strategy.NewBBR(
		withMockCPU(mcpu),
		strategy.WithCPUThreshold(500),
		strategy.WithWindow(200*time.Millisecond),
	)
	defer bb.Stop()

	// 预热：低 CPU 下建立 maxInFlight 基线
	var dones []func()
	for i := 0; i < 100; i++ {
		done, err := bb.Allow()
		if err != nil {
			t.Fatalf("预热阶段不应该拒绝: %v", err)
		}
		dones = append(dones, done)
	}
	for _, d := range dones {
		time.Sleep(time.Microsecond)
		d()
	}

	time.Sleep(300 * time.Millisecond)

	// 切换到高 CPU
	mcpu.set(900)

	var wg sync.WaitGroup
	var rejected int32
	total := 200
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := bb.Allow()
			if err != nil {
				atomic.AddInt32(&rejected, 1)
			}
		}()
	}
	wg.Wait()

	r := atomic.LoadInt32(&rejected)
	if r == 0 {
		t.Fatal("高 CPU 下应该有请求被拒绝")
	}
	t.Logf("高 CPU 拒绝率: %d/%d = %.1f%%", r, total, float64(r)/float64(total)*100)
}

func TestBBR_Cooldown(t *testing.T) {
	mcpu := &mockCPU{}
	mcpu.set(100)

	bb := strategy.NewBBR(
		withMockCPU(mcpu),
		strategy.WithCPUThreshold(500),
		strategy.WithWindow(200*time.Millisecond),
	)
	defer bb.Stop()

	for i := 0; i < 50; i++ {
		done, _ := bb.Allow()
		time.Sleep(time.Microsecond)
		done()
	}
	time.Sleep(300 * time.Millisecond)

	// 高 CPU 触发拒绝
	mcpu.set(900)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bb.Allow()
		}()
	}
	wg.Wait()

	// 降回低 CPU，1秒冷却期内继续部分限制
	mcpu.set(100)
	time.Sleep(100 * time.Millisecond)

	rejectedAfter := 0
	total := 100
	for i := 0; i < total; i++ {
		_, err := bb.Allow()
		if err != nil {
			rejectedAfter++
		}
	}
	t.Logf("冷却期拒绝率: %d/%d", rejectedAfter, total)

	// 等冷却期过
	time.Sleep(1100 * time.Millisecond)

	allPassed := true
	for i := 0; i < 100; i++ {
		_, err := bb.Allow()
		if err != nil {
			allPassed = false
			break
		}
	}
	if !allPassed {
		t.Fatal("冷却期过后低 CPU 下应该全部通过")
	}
}

func TestBBR_Options(t *testing.T) {
	bb := strategy.NewBBR(
		strategy.WithCPUThreshold(600),
		strategy.WithWindow(5*time.Second),
		strategy.WithDecay(0.9),
	)
	defer bb.Stop()

	mcpu := &mockCPU{}
	mcpu.set(100)
	bb.SetCPU(mcpu)

	done, err := bb.Allow()
	if err != nil {
		t.Fatal("低 CPU 请求应该通过")
	}
	done()
}

// ======================== 并发安全 ========================

func TestBBR_Concurrency(t *testing.T) {
	mcpu := &mockCPU{}
	mcpu.set(100)

	bb := strategy.NewBBR(
		withMockCPU(mcpu),
		strategy.WithWindow(200*time.Millisecond),
	)
	defer bb.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				done, err := bb.Allow()
				if err == nil {
					time.Sleep(time.Microsecond)
					done()
				}
			}
		}()
	}
	wg.Wait()
}

// ======================== Benchmark ========================

func BenchmarkBBR(b *testing.B) {
	mcpu := &mockCPU{}
	mcpu.set(100)
	bb := strategy.NewBBR(
		withMockCPU(mcpu),
		strategy.WithWindow(200*time.Millisecond),
	)
	defer bb.Stop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		done, err := bb.Allow()
		if err == nil {
			done()
		}
	}
}
