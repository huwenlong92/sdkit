package realtime

type Logger interface {
	Warn(msg string, fields ...any)
}

type nopLogger struct{}

func (nopLogger) Warn(string, ...any) {}
