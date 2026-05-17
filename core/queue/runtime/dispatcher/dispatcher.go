package dispatcher

import "github.com/huwenlong92/sdkit/core/queue"

type Dispatcher = queue.Dispatcher
type HandlerMetadata = queue.HandlerMetadata

func New() *Dispatcher {
	return queue.NewDispatcher()
}
