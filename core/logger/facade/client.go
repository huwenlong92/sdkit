package logger

import (
	"io"

	corelogger "github.com/huwenlong92/sdkit/core/logger"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type AsynqLogger = corelogger.AsynqLogger

func New(name string) (*zap.Logger, error) {
	return corelogger.New(name)
}

func Named(name string) *zap.Logger {
	return corelogger.Named(name)
}

func Writer(name, filename string) (io.Writer, error) {
	return corelogger.Writer(name, filename)
}

func WriteSyncer(name, filename string) (zapcore.WriteSyncer, error) {
	return corelogger.WriteSyncer(name, filename)
}

func Asynq(name string) *AsynqLogger {
	return corelogger.Asynq(name)
}

func Cron(name string) cron.Logger {
	return corelogger.Cron(name)
}
