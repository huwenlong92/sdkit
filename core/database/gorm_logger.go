package database

import (
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"

	gormLogger "gorm.io/gorm/logger"
)

func newGORMLogger(level string, mode string) gormLogger.Interface {
	logLevel := parseGORMLogLevel(level, mode)
	if logLevel == gormLogger.Silent {
		return gormLogger.New(log.New(io.Discard, "\r\n", log.LstdFlags), gormLogger.Config{LogLevel: logLevel})
	}

	writer, err := logger.Writer("gorm", "runtime.log")
	if err != nil {
		writer = io.Discard
	}
	if mode == "dev" {
		writer = io.MultiWriter(os.Stdout, writer)
	}

	return gormLogger.New(
		log.New(writer, "\r\n", log.LstdFlags),
		gormLogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: false,
			Colorful:                  false,
		},
	)
}

func parseGORMLogLevel(level, mode string) gormLogger.LogLevel {
	switch strings.ToLower(level) {
	case "silent", "none":
		return gormLogger.Silent
	case "error":
		return gormLogger.Error
	case "info", "debug":
		return gormLogger.Info
	case "warn":
		return gormLogger.Warn
	default:
		if mode == "dev" {
			return gormLogger.Info
		}
		return gormLogger.Warn
	}
}
