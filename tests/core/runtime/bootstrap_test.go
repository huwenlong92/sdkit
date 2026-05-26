package tests

import (
	"testing"

	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestBootstrapCapabilityConstants(t *testing.T) {
	if runtime.CapabilityBootstrap != "bootstrap" {
		t.Fatalf("CapabilityBootstrap = %q, want bootstrap", runtime.CapabilityBootstrap)
	}
	if runtime.CapabilityBootstrapError != "bootstrap.error" {
		t.Fatalf("CapabilityBootstrapError = %q, want bootstrap.error", runtime.CapabilityBootstrapError)
	}
}

func TestBootstrapDependencies(t *testing.T) {
	dep := runtime.OptionalBootstrap()
	if dep.Name != runtime.CapabilityBootstrap {
		t.Fatalf("OptionalBootstrap().Name = %q, want %q", dep.Name, runtime.CapabilityBootstrap)
	}
	if dep.Required {
		t.Fatal("OptionalBootstrap() should be optional")
	}

	dep = runtime.RequireBootstrap()
	if dep.Name != runtime.CapabilityBootstrap {
		t.Fatalf("RequireBootstrap().Name = %q, want %q", dep.Name, runtime.CapabilityBootstrap)
	}
	if !dep.Required {
		t.Fatal("RequireBootstrap() should be required")
	}
}
