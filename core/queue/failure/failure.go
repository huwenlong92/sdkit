package failure

import "github.com/huwenlong92/sdkit/core/queue"

type Logger = queue.FailureLogger
type LogConfig = queue.FailureLogConfig
type Writer = queue.FailureWriter

func NewLogger(writer Writer, cfg LogConfig) *Logger {
	return queue.NewFailureLogger(writer, cfg)
}

func LogHandler(message string) queue.FailureHandler {
	return queue.LogFailureHandler(message)
}
