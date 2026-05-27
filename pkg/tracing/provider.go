package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/huwenlong92/sdkit/core/tracecontext"
)

var ErrNotCompiled = errors.New("tracing enabled but binary was built without sdkit_tracing_otel")

type Provider interface {
	Init(context.Context, Config) (func(context.Context) error, error)
	StartSpan(context.Context, string, SpanOptions, ...Attr) (context.Context, Span)
	InjectCarrier(context.Context, tracecontext.Carrier)
	ExtractCarrier(context.Context, tracecontext.Carrier) context.Context
}

var (
	providerMu     sync.RWMutex
	providers               = map[string]Provider{}
	globalProvider Provider = noopProvider{}
	globalShutdown          = noopShutdown
	shutdownMu     sync.Mutex
	enabled        atomic.Bool
)

func RegisterProvider(name string, provider Provider) {
	name = strings.TrimSpace(name)
	if name == "" || provider == nil {
		return
	}
	providerMu.Lock()
	providers[name] = provider
	providerMu.Unlock()
}

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	cfg = NormalizeConfig(cfg)
	provider := resolveProvider("otel")
	if provider == nil {
		provider = noopProvider{}
	}
	shutdown, err := provider.Init(ctx, cfg)
	if err != nil {
		enabled.Store(false)
		if cfg.Enabled {
			return nil, err
		}
		shutdown = noopShutdown
	}
	if shutdown == nil {
		shutdown = noopShutdown
	}
	setProvider(provider)
	setShutdown(shutdown)
	enabled.Store(cfg.Enabled && err == nil)
	return shutdown, nil
}

func Enabled() bool {
	return enabled.Load()
}

func Shutdown(ctx context.Context) error {
	shutdownMu.Lock()
	shutdown := globalShutdown
	globalShutdown = noopShutdown
	shutdownMu.Unlock()
	enabled.Store(false)
	setProvider(noopProvider{})
	return shutdown(ctx)
}

func currentProvider() Provider {
	providerMu.RLock()
	provider := globalProvider
	providerMu.RUnlock()
	if provider == nil {
		return noopProvider{}
	}
	return provider
}

func setProvider(provider Provider) {
	if provider == nil {
		provider = noopProvider{}
	}
	providerMu.Lock()
	globalProvider = provider
	providerMu.Unlock()
}

func resolveProvider(name string) Provider {
	providerMu.RLock()
	provider := providers[name]
	providerMu.RUnlock()
	return provider
}

func setShutdown(shutdown func(context.Context) error) {
	if shutdown == nil {
		shutdown = noopShutdown
	}
	shutdownMu.Lock()
	globalShutdown = shutdown
	shutdownMu.Unlock()
}

func noopShutdown(context.Context) error {
	return nil
}

type noopProvider struct{}

func (noopProvider) Init(_ context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.Enabled {
		return nil, ErrNotCompiled
	}
	return noopShutdown, nil
}

func (noopProvider) StartSpan(ctx context.Context, _ string, _ SpanOptions, _ ...Attr) (context.Context, Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	traceID := tracecontext.TraceID(ctx)
	if traceID == "" {
		traceID = randomHex(16)
	}
	spanID := randomHex(8)
	ctx = tracecontext.ContextWithTrace(ctx, traceID, spanID)
	return ctx, noopSpan{traceID: traceID, spanID: spanID}
}

func (noopProvider) InjectCarrier(ctx context.Context, carrier tracecontext.Carrier) {
	tracecontext.InjectCarrier(ctx, carrier)
}

func (noopProvider) ExtractCarrier(ctx context.Context, carrier tracecontext.Carrier) context.Context {
	return tracecontext.ExtractCarrier(ctx, carrier)
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
