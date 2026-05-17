package tests

import (
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/ratelimit/strategy"
)

// ======================== TokenBucket ========================

func TestTokenBucket_Allow(t *testing.T) {
	tb := strategy.NewTokenBucket(10, 2)

	if !tb.Allow("127.0.0.1") {
		t.Fatal("第一个请求应该通过")
	}
	if !tb.Allow("127.0.0.1") {
		t.Fatal("第二个请求应该通过（突发）")
	}
	if tb.Allow("127.0.0.1") {
		t.Fatal("第三个请求应该被限（突发已用完）")
	}
}

func TestTokenBucket_PerKey(t *testing.T) {
	tb := strategy.NewTokenBucket(100, 1)

	if !tb.Allow("192.168.1.1") {
		t.Fatal("IP1 第1次应该通过")
	}
	if !tb.Allow("192.168.1.2") {
		t.Fatal("IP2 第1次应该通过")
	}
	if tb.Allow("192.168.1.1") {
		t.Fatal("IP1 第2次应该被限（突发=1）")
	}
	if tb.Allow("192.168.1.2") {
		t.Fatal("IP2 第2次也应该被限（突发=1）")
	}
}

func TestTokenBucket_AllowN(t *testing.T) {
	tb := strategy.NewTokenBucket(100, 10)

	if !tb.AllowN("10.0.0.1", 5) {
		t.Fatal("AllowN(5) 应该通过（突发=10）")
	}
	if tb.AllowN("10.0.0.1", 6) {
		t.Fatal("AllowN(6) 应该被限（5+6>10）")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := strategy.NewTokenBucket(100, 1)

	if !tb.Allow("10.0.0.1") {
		t.Fatal("第1个请求应该通过")
	}
	if tb.Allow("10.0.0.1") {
		t.Fatal("第2个请求应该被限（突发=1）")
	}
	time.Sleep(20 * time.Millisecond)
	if !tb.Allow("10.0.0.1") {
		t.Fatal("20ms 后应该有令牌恢复")
	}
}

// ======================== SlidingWindow ========================

func TestSlidingWindow_Allow(t *testing.T) {
	sw := strategy.NewSlidingWindow(100*time.Millisecond, 3)

	if !sw.Allow("127.0.0.1") {
		t.Fatal("第1个请求应该通过")
	}
	if !sw.Allow("127.0.0.1") {
		t.Fatal("第2个请求应该通过")
	}
	if !sw.Allow("127.0.0.1") {
		t.Fatal("第3个请求应该通过")
	}
	if sw.Allow("127.0.0.1") {
		t.Fatal("第4个请求应该被限（窗口内已达上限）")
	}
}

func TestSlidingWindow_PerKey(t *testing.T) {
	sw := strategy.NewSlidingWindow(time.Second, 2)

	if !sw.Allow("192.168.1.1") {
		t.Fatal("IP1 第1次应该通过")
	}
	if !sw.Allow("192.168.1.2") {
		t.Fatal("IP2 第1次应该通过")
	}
	if !sw.Allow("192.168.1.1") {
		t.Fatal("IP1 第2次应该通过（limit=2）")
	}
	if sw.Allow("192.168.1.1") {
		t.Fatal("IP1 第3次应该被限")
	}
	if !sw.Allow("192.168.1.2") {
		t.Fatal("IP2 第2次应该通过（limit=2）")
	}
}

func TestSlidingWindow_Expiry(t *testing.T) {
	sw := strategy.NewSlidingWindow(50*time.Millisecond, 2)

	sw.Allow("10.0.0.1")
	sw.Allow("10.0.0.1")
	if sw.Allow("10.0.0.1") {
		t.Fatal("第3个请求应该被限")
	}

	time.Sleep(60 * time.Millisecond)

	if !sw.Allow("10.0.0.1") {
		t.Fatal("窗口过期后应该通过")
	}
}

func TestSlidingWindow_AllowN(t *testing.T) {
	sw := strategy.NewSlidingWindow(200*time.Millisecond, 5)

	if !sw.AllowN("10.0.0.1", 3) {
		t.Fatal("AllowN(3) 应该通过（3<=5）")
	}
	if !sw.AllowN("10.0.0.1", 2) {
		t.Fatal("AllowN(2) 应该通过（3+2=5）")
	}
	if sw.AllowN("10.0.0.1", 1) {
		t.Fatal("AllowN(1) 应该被限（已达上限）")
	}
}

// ======================== 并发安全 ========================

func TestTokenBucket_Concurrency(t *testing.T) {
	tb := strategy.NewTokenBucket(1000, 100)
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tb.Allow("concurrent-key")
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

func TestSlidingWindow_Concurrency(t *testing.T) {
	sw := strategy.NewSlidingWindow(time.Second, 5000)
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				sw.AllowN("concurrent-key", 1)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

// ======================== FixedWindow ========================

func TestFixedWindow_Allow(t *testing.T) {
	fw := strategy.NewFixedWindow(100*time.Millisecond, 3)

	if !fw.Allow("127.0.0.1") {
		t.Fatal("第1个请求应该通过")
	}
	if !fw.Allow("127.0.0.1") {
		t.Fatal("第2个请求应该通过")
	}
	if !fw.Allow("127.0.0.1") {
		t.Fatal("第3个请求应该通过")
	}
	if fw.Allow("127.0.0.1") {
		t.Fatal("第4个请求应该被限（窗口内已达上限）")
	}
}

func TestFixedWindow_PerKey(t *testing.T) {
	fw := strategy.NewFixedWindow(time.Second, 2)

	if !fw.Allow("192.168.1.1") {
		t.Fatal("IP1 第1次应该通过")
	}
	if !fw.Allow("192.168.1.2") {
		t.Fatal("IP2 第1次应该通过")
	}
	if !fw.Allow("192.168.1.1") {
		t.Fatal("IP1 第2次应该通过（limit=2）")
	}
	if fw.Allow("192.168.1.1") {
		t.Fatal("IP1 第3次应该被限")
	}
	if !fw.Allow("192.168.1.2") {
		t.Fatal("IP2 第2次应该通过（limit=2）")
	}
}

func TestFixedWindow_Expiry(t *testing.T) {
	fw := strategy.NewFixedWindow(50*time.Millisecond, 2)

	fw.Allow("10.0.0.1")
	fw.Allow("10.0.0.1")
	if fw.Allow("10.0.0.1") {
		t.Fatal("第3个请求应该被限")
	}

	// 等当前窗口过期，进入新窗口
	time.Sleep(60 * time.Millisecond)

	if !fw.Allow("10.0.0.1") {
		t.Fatal("新窗口应该通过")
	}
}

func TestFixedWindow_CrossWindow(t *testing.T) {
	fw := strategy.NewFixedWindow(30*time.Millisecond, 2)

	fw.Allow("10.0.0.1")
	fw.Allow("10.0.0.1")

	// 进入新窗口，计数应重置
	time.Sleep(40 * time.Millisecond)

	if !fw.Allow("10.0.0.1") {
		t.Fatal("新窗口第1个应该通过（计数已重置）")
	}
	if !fw.Allow("10.0.0.1") {
		t.Fatal("新窗口第2个应该通过")
	}
	if fw.Allow("10.0.0.1") {
		t.Fatal("新窗口第3个应该被限")
	}
}

func TestFixedWindow_AllowN(t *testing.T) {
	fw := strategy.NewFixedWindow(200*time.Millisecond, 5)

	if !fw.AllowN("10.0.0.1", 3) {
		t.Fatal("AllowN(3) 应该通过（3<=5）")
	}
	if !fw.AllowN("10.0.0.1", 2) {
		t.Fatal("AllowN(2) 应该通过（3+2=5）")
	}
	if fw.AllowN("10.0.0.1", 1) {
		t.Fatal("AllowN(1) 应该被限（已达上限）")
	}
}

func TestFixedWindow_Concurrency(t *testing.T) {
	fw := strategy.NewFixedWindow(time.Second, 5000)
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				fw.AllowN("concurrent-key", 1)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

// ======================== LeakyBucket ========================

func TestLeakyBucket_Allow(t *testing.T) {
	// rate=100/s, capacity=3：前3个瞬间入桶，之后被限
	lb := strategy.NewLeakyBucket(100, 3)

	if !lb.Allow("127.0.0.1") {
		t.Fatal("第1个请求应该通过")
	}
	if !lb.Allow("127.0.0.1") {
		t.Fatal("第2个请求应该通过")
	}
	if !lb.Allow("127.0.0.1") {
		t.Fatal("第3个请求应该通过")
	}
	if lb.Allow("127.0.0.1") {
		t.Fatal("第4个请求应该被限（容量已满）")
	}
}

func TestLeakyBucket_PerKey(t *testing.T) {
	lb := strategy.NewLeakyBucket(100, 2)

	if !lb.Allow("192.168.1.1") {
		t.Fatal("IP1 第1次应该通过")
	}
	if !lb.Allow("192.168.1.2") {
		t.Fatal("IP2 第1次应该通过")
	}
	if !lb.Allow("192.168.1.1") {
		t.Fatal("IP1 第2次应该通过（capacity=2）")
	}
	if lb.Allow("192.168.1.1") {
		t.Fatal("IP1 第3次应该被限")
	}
	if !lb.Allow("192.168.1.2") {
		t.Fatal("IP2 第2次应该通过（独立于 IP1）")
	}
}

func TestLeakyBucket_Refill(t *testing.T) {
	// rate=100/s → 每10ms漏1个
	lb := strategy.NewLeakyBucket(100, 2)

	lb.Allow("10.0.0.1")
	lb.Allow("10.0.0.1")
	if lb.Allow("10.0.0.1") {
		t.Fatal("容量满，应该被限")
	}

	// 等漏出
	time.Sleep(20 * time.Millisecond)

	if !lb.Allow("10.0.0.1") {
		t.Fatal("漏出后应该通过")
	}
}

func TestLeakyBucket_Concurrency(t *testing.T) {
	lb := strategy.NewLeakyBucket(1e6, 1e6)
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				lb.Allow("concurrent-key")
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

// ======================== Benchmark ========================

func BenchmarkLeakyBucket(b *testing.B) {
	lb := strategy.NewLeakyBucket(1e6, 1e6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.Allow("127.0.0.1")
	}
}

func BenchmarkTokenBucket(b *testing.B) {
	tb := strategy.NewTokenBucket(1e6, 1e6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.Allow("127.0.0.1")
	}
}

func BenchmarkSlidingWindow(b *testing.B) {
	sw := strategy.NewSlidingWindow(time.Second, 1e6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw.Allow("127.0.0.1")
	}
}

func BenchmarkFixedWindow(b *testing.B) {
	fw := strategy.NewFixedWindow(time.Second, 1e6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fw.Allow("127.0.0.1")
	}
}
