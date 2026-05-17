package redisx

type Config struct {
	Addr     string `mapstructure:"addr" yaml:"addr"`
	Username string `mapstructure:"username" yaml:"username"`
	Password string `mapstructure:"password" yaml:"password"`
	DB       int    `mapstructure:"db" yaml:"db"`

	Prefix string `mapstructure:"prefix" yaml:"prefix"`

	PoolSize     int `mapstructure:"pool_size" yaml:"pool_size"`
	MinIdleConns int `mapstructure:"min_idle_conns" yaml:"min_idle_conns"`
}
