package tracing

import (
	"github.com/huwenlong92/sdkit/core/runtime"
	coretracing "github.com/huwenlong92/sdkit/core/tracing"
)

const Name = "tracing"

type Config = coretracing.Config

type ConfigLoader func(app *runtime.App) (Config, error)
