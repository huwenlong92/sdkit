package database

import (
	"context"
	"strings"

	"github.com/huwenlong92/sdkit/core/logger"

	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/zap"
)

type pgxLogger struct {
	l *zap.Logger
}

func newPGXLogger() tracelog.Logger {
	return pgxLogger{l: logger.Named("pgx")}
}

func (l pgxLogger) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	fields := make([]zap.Field, 0, len(data)+1)
	for k, v := range data {
		fields = append(fields, zap.Any(k, v))
	}
	fields = append(fields, zap.String("component", "pgx"))
	fields = append(fields, logger.ContextFields(ctx)...)

	switch level {
	case tracelog.LogLevelTrace, tracelog.LogLevelDebug:
		l.l.Debug(msg, fields...)
	case tracelog.LogLevelInfo:
		l.l.Info(msg, fields...)
	case tracelog.LogLevelWarn:
		l.l.Warn(msg, fields...)
	case tracelog.LogLevelError:
		l.l.Error(msg, fields...)
	}
}

func pgxLogLevel(level, mode string) tracelog.LogLevel {
	switch strings.ToLower(level) {
	case "silent", "none":
		return tracelog.LogLevelNone
	case "error":
		return tracelog.LogLevelError
	case "warn":
		return tracelog.LogLevelWarn
	case "info":
		return tracelog.LogLevelInfo
	case "debug":
		return tracelog.LogLevelDebug
	case "trace":
		return tracelog.LogLevelTrace
	default:
		if mode == "dev" {
			return tracelog.LogLevelInfo
		}
		return tracelog.LogLevelWarn
	}
}
