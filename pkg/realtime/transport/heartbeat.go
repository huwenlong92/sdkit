package transport

import "time"

type HeartbeatConfig struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PingInterval time.Duration
}

func (c HeartbeatConfig) Normalize() HeartbeatConfig {
	if c.ReadTimeout < 0 {
		c.ReadTimeout = 0
	}
	if c.WriteTimeout < 0 {
		c.WriteTimeout = 0
	}
	if c.PingInterval < 0 {
		c.PingInterval = 0
	}
	return c
}
