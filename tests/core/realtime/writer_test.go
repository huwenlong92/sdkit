package realtime_test

import (
	"io"
	"testing"

	"github.com/huwenlong92/sdkit/core/realtime"
)

func TestEventWriterInterface(t *testing.T) {
	var _ realtime.EventWriter = eventWriterFunc(func(_ realtime.Event) error {
		return nil
	})
}

type eventWriterFunc func(realtime.Event) error

func (f eventWriterFunc) WriteEvent(_ io.Writer, event realtime.Event) error {
	return f(event)
}
