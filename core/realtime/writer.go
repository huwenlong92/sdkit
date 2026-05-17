package realtime

import "io"

type EventWriter interface {
	WriteEvent(w io.Writer, event Event) error
}
