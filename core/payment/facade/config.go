package payment

import (
	coreconfig "github.com/huwenlong92/sdkit/core/config"
	"github.com/huwenlong92/sdkit/core/runtime"
)

type Config struct {
	Channels        []ChannelBinding `mapstructure:"channels" yaml:"channels"`
	ChannelBindings []ChannelBinding `mapstructure:"channel_bindings" yaml:"channel_bindings"`
}

type ConfigLoader func(app *runtime.App) (Config, error)

func (c Config) channelBindings() []ChannelBinding {
	total := len(c.Channels) + len(c.ChannelBindings)
	if total == 0 {
		return nil
	}
	bindings := make([]ChannelBinding, 0, total)
	bindings = append(bindings, c.Channels...)
	bindings = append(bindings, c.ChannelBindings...)
	return bindings
}

func loadConfigFromCore(*runtime.App) (Config, error) {
	if coreconfig.V == nil || !coreconfig.V.IsSet("payment") {
		return Config{}, nil
	}
	var cfg Config
	if err := coreconfig.V.UnmarshalKey("payment", &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
