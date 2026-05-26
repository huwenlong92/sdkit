package tests

import (
	"context"
	"reflect"
	"testing"

	"github.com/huwenlong92/sdkit/core/cache"
	cachefacade "github.com/huwenlong92/sdkit/core/cache/facade"
	"github.com/huwenlong92/sdkit/core/database"
	databasefacade "github.com/huwenlong92/sdkit/core/database/facade"
	"github.com/huwenlong92/sdkit/core/logger"
	loggerfacade "github.com/huwenlong92/sdkit/core/logger/facade"
	coreredis "github.com/huwenlong92/sdkit/core/redis"
	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"

	"go.uber.org/zap"
)

func TestPhase2CapabilityOrderIsStable(t *testing.T) {
	app := runtime.New()
	var calls []string
	if err := app.Use(
		runtime.NewCapability("logger", func(*runtime.App) error {
			calls = append(calls, "logger")
			return nil
		}),
		runtime.NewCapability("database", func(*runtime.App) error {
			calls = append(calls, "database")
			return nil
		}),
		runtime.NewCapability("redis", func(*runtime.App) error {
			calls = append(calls, "redis")
			return nil
		}),
	); err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []string{"logger", "database", "redis"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("capability calls = %v, want %v", calls, want)
	}
}

func TestPhase2PlatformCapabilitiesBindResources(t *testing.T) {
	app := runtime.New()
	log := zap.NewNop()
	db := &database.Database{}
	redisClient := &coreredis.RuntimeClient{}
	t.Cleanup(func() {
		cache.Close()
		_ = coreredis.Close()
		database.Close()
	})

	if err := app.Use(
		loggerfacade.Use(loggerfacade.WithLogger(log)),
		databasefacade.Use(databasefacade.WithDatabase(db)),
		redisfacade.Use(redisfacade.WithClient(redisClient)),
		cachefacade.Use(cachefacade.WithConfig(cachefacade.Config{Prefix: "runtime-test:"})),
	); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := logger.From(app); got != log {
		t.Fatalf("logger.From() = %p, want %p", got, log)
	}
	if got := database.From(app); got != db {
		t.Fatalf("database.From() = %p, want %p", got, db)
	}
	if got := coreredis.From(app); got != redisClient {
		t.Fatalf("redis.From() = %p, want %p", got, redisClient)
	}
	if got := cache.From(app); got == nil {
		t.Fatal("cache.From() = nil, want cache instance")
	}
}

func TestPhase2ProvidersShareRuntimeCapabilityInstance(t *testing.T) {
	app := runtime.New()
	db := &database.Database{}
	t.Cleanup(database.Close)

	if err := app.Use(databasefacade.Use(databasefacade.WithDatabase(db))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	var providers []*database.Database
	if err := app.Register(
		testProvider{
			name: "api",
			register: func(app *runtime.App) error {
				providers = append(providers, database.From(app))
				return nil
			},
		},
		testProvider{
			name: "worker",
			register: func(app *runtime.App) error {
				providers = append(providers, database.From(app))
				return nil
			},
		},
	); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("provider count = %d, want 2", len(providers))
	}
	if providers[0] != db || providers[1] != db {
		t.Fatalf("providers did not share database instance: %p %p want %p", providers[0], providers[1], db)
	}
}
