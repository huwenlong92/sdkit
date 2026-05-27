package tracing

import pkgtracing "github.com/huwenlong92/sdkit/pkg/tracing"

const defaultServiceName = "sdkitgo"

type Config = pkgtracing.Config

func DefaultConfig() Config {
	return pkgtracing.DefaultConfig()
}

func normalizeConfig(cfg Config) Config {
	return pkgtracing.NormalizeConfig(cfg)
}
