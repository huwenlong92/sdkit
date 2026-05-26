package facade_test

import (
	"context"
	"errors"
	"testing"

	coreredis "github.com/huwenlong92/sdkit/core/redis"
	redisfacade "github.com/huwenlong92/sdkit/core/redis/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestUseDefaultsToInternal(t *testing.T) {
	capability := redisfacade.Use()
	if !capability.Metadata().Internal {
		t.Fatal("Use() should default to internal capability")
	}

	capability = redisfacade.Use(redisfacade.WithExternal())
	if capability.Metadata().Internal {
		t.Fatal("WithExternal() should expose capability")
	}
}

func TestUseRequiresConfigOrClient(t *testing.T) {
	app := runtime.New()
	if err := app.Use(redisfacade.Use()); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); !errors.Is(err, redisfacade.ErrConfigRequired) {
		t.Fatalf("Run() error = %v, want ErrConfigRequired", err)
	}
}

func TestUseWithClientSkipsConfig(t *testing.T) {
	t.Cleanup(func() {
		_ = coreredis.Close()
	})

	client := &coreredis.RuntimeClient{}
	app := runtime.New()
	if err := app.Use(redisfacade.Use(redisfacade.WithClient(client))); err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := coreredis.From(app); got != client {
		t.Fatalf("redis.From(app) = %p, want %p", got, client)
	}
}
