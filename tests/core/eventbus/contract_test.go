package tests

import (
	"github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/pkg/eventbus/memory"
)

var (
	_ eventbus.Service = memory.New()
	_ eventbus.Bus     = memory.New()
)
