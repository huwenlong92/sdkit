package logger

import (
	corelogger "github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/runtime"

	"go.uber.org/zap"
)

func Init(name, level, mode string) error {
	return corelogger.Init(name, level, mode)
}

func Configure(cfg Config) error {
	return corelogger.Configure(cfg)
}

func From(app *runtime.App) *zap.Logger {
	return corelogger.From(app)
}

func Default() *zap.Logger {
	return corelogger.Default()
}

func Debug(msg string, fields ...zap.Field) {
	corelogger.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	corelogger.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	corelogger.Warn(msg, fields...)
}

func Error(msg string, err error, fields ...zap.Field) {
	corelogger.Error(msg, err, fields...)
}

func Sync() {
	corelogger.Sync()
}
