package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
	coresession "github.com/huwenlong92/sdkit/core/session"

	"github.com/gin-gonic/gin"
)

func TestUseRegistersSessionCapability(t *testing.T) {
	capability := Use(WithConfig(Config{Prefix: "session-test:"}))

	metadata := capability.Metadata()
	if metadata.Name != string(KeySession) {
		t.Fatalf("metadata name = %q, want %q", metadata.Name, KeySession)
	}
	if metadata.Group != runtime.GroupSystem {
		t.Fatalf("metadata group = %q, want %q", metadata.Group, runtime.GroupSystem)
	}
	if metadata.Scope != runtime.ScopeGlobal {
		t.Fatalf("metadata scope = %q, want %q", metadata.Scope, runtime.ScopeGlobal)
	}

	app := runtime.New()
	if err := app.RegisterCapabilities(capability); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := From(app); got == nil {
		t.Fatal("From(app) = nil, want store")
	}
}

func TestUseConfigLoaderRunsAfterBootstrap(t *testing.T) {
	app := runtime.New()
	order := make([]string, 0, 2)
	if err := app.RegisterCapabilities(
		Use(WithConfigLoader(func(app *runtime.App) (Config, error) {
			if _, ok := app.Container().Get(runtime.Key("config")); !ok {
				t.Fatal("config not initialized before session config loader")
			}
			order = append(order, "session.loader")
			return Config{Prefix: "session-loader:"}, nil
		})),
		runtime.NewCapability("bootstrap", func(app *runtime.App) error {
			order = append(order, "bootstrap")
			return app.Container().Bind(runtime.Key("config"), true)
		}),
	); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"bootstrap", "session.loader"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestUseServiceLocalNameBindsStore(t *testing.T) {
	capability := Use(WithName("api.session"), WithConfig(Config{Prefix: "api-session:"}))
	if metadata := capability.Metadata(); metadata.Name != "api.session" {
		t.Fatalf("metadata name = %q, want api.session", metadata.Name)
	}

	app := runtime.New()
	if err := app.RegisterCapabilities(capability); err != nil {
		t.Fatalf("RegisterCapabilities() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	value, ok := app.Container().Get(runtime.Key("api.session"))
	if !ok {
		t.Fatal("api.session was not bound")
	}
	if _, ok := value.(Store); !ok {
		t.Fatalf("api.session = %T, want session.Store", value)
	}
}

func TestFromServiceContext(t *testing.T) {
	store := coresession.NewMemoryStore()
	ctx := testServiceContext{values: map[string]any{Name: store}}

	got, ok := FromServiceContext(ctx)
	if !ok || got != store {
		t.Fatalf("FromServiceContext() = %#v, %v; want store, true", got, ok)
	}
}

func TestMiddlewareFromServiceContextWithoutStoreIsNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MiddlewareFromServiceContext(testServiceContext{}))
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
}

type testServiceContext struct {
	values map[string]any
}

func (ctx testServiceContext) CapabilityLocalFirst(name string) (any, bool) {
	value, ok := ctx.values[name]
	return value, ok
}
