//go:build sdkit_eventbus_nats

package eventbus

import (
	"strings"

	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/pkg/eventbus/nats"
)

func init() {
	registerDriverFactory(DriverNATS, func(cfg Config, _ options) (coreeventbus.Bus, error) {
		subjectPrefix := cfg.SubjectPrefix
		if subjectPrefix == "" {
			subjectPrefix = strings.ReplaceAll(cfg.TopicPrefix, ":", ".")
		}
		return nats.New(cfg.Addr, subjectPrefix)
	})
}
