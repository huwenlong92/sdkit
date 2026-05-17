package tests

import (
	"github.com/huwenlong92/sdkit/core/eventbus"
	eventbusmemory "github.com/huwenlong92/sdkit/pkg/eventbus/memory"
)

var (
	_ eventbus.Service = eventbusmemory.New()
	_ eventbus.Bus     = eventbusmemory.New()
)
