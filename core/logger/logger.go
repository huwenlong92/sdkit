package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/core/runtime"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const KeyLogger runtime.Key = "logger"

const (
	defaultRootDir    = "logs"
	defaultRotateMode = "size"
	defaultMaxSize    = 10
	defaultMaxBackups = 5
	defaultMaxAge     = 30
)

type RotationConfig struct {
	Mode       string `mapstructure:"mode" yaml:"mode"`
	MaxSize    int    `mapstructure:"max_size" yaml:"max_size"`
	MaxBackups int    `mapstructure:"max_backups" yaml:"max_backups"`
	MaxAge     int    `mapstructure:"max_age" yaml:"max_age"`
	Compress   bool   `mapstructure:"compress" yaml:"compress"`
}

type Config struct {
	Name     string         `mapstructure:"name" yaml:"name"`
	Level    string         `mapstructure:"level" yaml:"level"`
	Mode     string         `mapstructure:"mode" yaml:"mode"`
	Format   string         `mapstructure:"format" yaml:"format"`
	RootDir  string         `mapstructure:"root_dir" yaml:"root_dir"`
	Rotation RotationConfig `mapstructure:"rotation" yaml:"rotation"`
}

var (
	L      *zap.Logger
	global Config
)

func Init(name, level, mode string) error {
	return Configure(Config{Name: name, Level: level, Mode: mode})
}

func Configure(cfg Config) error {
	cfg = normalizeConfig(cfg)
	global = cfg

	l, err := New(cfg.Name)
	if err != nil {
		return err
	}
	L = l
	return nil
}

func New(name string) (*zap.Logger, error) {
	cfg := global
	cfg.Name = name
	cfg = normalizeConfig(cfg)

	encoder := newEncoder(cfg)
	writeSyncer, err := WriteSyncer(name, name+".log")
	if err != nil {
		return nil, err
	}

	core := zapcore.NewCore(encoder, writeSyncer, zap.NewAtomicLevelAt(parseZapLevel(cfg.Level)))
	return zap.New(core, zap.AddCaller()), nil
}

func Named(name string) *zap.Logger {
	l, err := New(name)
	if err != nil {
		if L != nil {
			return L
		}
		return zap.NewNop()
	}
	return l
}

func Writer(name, filename string) (io.Writer, error) {
	cfg := normalizeConfig(global)
	dir := filepath.Join(cfg.RootDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	if cfg.Rotation.Mode == "daily" {
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filepath.Base(filename), ext)
		if base == "" {
			base = name
		}
		return newDailyWriter(dailyWriterConfig{
			Dir:    dir,
			Name:   base,
			Ext:    ext,
			MaxAge: cfg.Rotation.MaxAge,
			Now:    time.Now,
		}), nil
	}
	return &lumberjack.Logger{
		Filename:   filepath.Join(dir, filename),
		MaxSize:    cfg.Rotation.MaxSize,
		MaxBackups: cfg.Rotation.MaxBackups,
		MaxAge:     cfg.Rotation.MaxAge,
		Compress:   cfg.Rotation.Compress,
	}, nil
}

func WriteSyncer(name, filename string) (zapcore.WriteSyncer, error) {
	fileWriter, err := Writer(name, filename)
	if err != nil {
		return nil, err
	}

	writers := []zapcore.WriteSyncer{zapcore.AddSync(fileWriter)}
	if normalizeConfig(global).Mode == "dev" {
		writers = append([]zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}, writers...)
	} else {
		writers = append([]zapcore.WriteSyncer{zapcore.AddSync(os.Stderr)}, writers...)
	}
	return zapcore.NewMultiWriteSyncer(writers...), nil
}

type AsynqLogger struct {
	logger *zap.SugaredLogger
}

func Asynq(name string) *AsynqLogger {
	return &AsynqLogger{logger: Named(name).Sugar()}
}

func (l *AsynqLogger) Debug(args ...interface{}) { l.logger.Debug(args...) }
func (l *AsynqLogger) Info(args ...interface{})  { l.logger.Info(args...) }
func (l *AsynqLogger) Warn(args ...interface{})  { l.logger.Warn(args...) }
func (l *AsynqLogger) Error(args ...interface{}) { l.logger.Error(args...) }
func (l *AsynqLogger) Fatal(args ...interface{}) { l.logger.Fatal(args...) }

func Cron(name string) cron.Logger {
	return cronLogger{logger: Named(name)}
}

type cronLogger struct {
	logger *zap.Logger
}

func (l cronLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Info(msg, zapFields(keysAndValues...)...)
}

func (l cronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	fields := append([]zap.Field{zap.Error(err)}, zapFields(keysAndValues...)...)
	l.logger.Error(msg, fields...)
}

func From(app *runtime.App) *zap.Logger {
	if app != nil {
		if value, ok := app.Container().Get(KeyLogger); ok {
			if log, ok := value.(*zap.Logger); ok {
				return log
			}
		}
	}
	if L != nil {
		return L
	}
	return zap.NewNop()
}

func Default() *zap.Logger {
	return From(nil)
}

func Debug(msg string, fields ...zap.Field) {
	Default().Debug(msg, withTraceID(fields)...)
}

func Info(msg string, fields ...zap.Field) {
	Default().Info(msg, withTraceID(fields)...)
}

func Warn(msg string, fields ...zap.Field) {
	Default().Warn(msg, withTraceID(fields)...)
}

func Error(msg string, err error, fields ...zap.Field) {
	if !hasErrorField(fields) {
		fields = append([]zap.Field{zap.Error(err)}, fields...)
	}
	Default().Error(msg, withTraceID(fields)...)
}

func Sync() {
	if L != nil {
		_ = L.Sync()
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.Name == "" {
		cfg.Name = "app"
	}
	if cfg.Level == "" {
		cfg.Level = "info"
	}
	if cfg.RootDir == "" {
		cfg.RootDir = defaultRootDir
	}
	cfg.Rotation.Mode = strings.ToLower(strings.TrimSpace(cfg.Rotation.Mode))
	if cfg.Rotation.Mode == "" {
		cfg.Rotation.Mode = defaultRotateMode
	}
	if cfg.Rotation.Mode != "daily" {
		cfg.Rotation.Mode = defaultRotateMode
	}
	if cfg.Rotation.MaxSize <= 0 {
		cfg.Rotation.MaxSize = defaultMaxSize
	}
	if cfg.Rotation.MaxBackups <= 0 {
		cfg.Rotation.MaxBackups = defaultMaxBackups
	}
	if cfg.Rotation.MaxAge <= 0 {
		cfg.Rotation.MaxAge = defaultMaxAge
	}
	return cfg
}

func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
}

func newEncoder(cfg Config) zapcore.Encoder {
	if cfg.Mode == "dev" && cfg.Format != "json" {
		encoderConfig := zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeTime = customTimeEncoder
		return zapcore.NewConsoleEncoder(encoderConfig)
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = customTimeEncoder
	if cfg.Format == "console" {
		return zapcore.NewConsoleEncoder(encoderConfig)
	}
	return zapcore.NewJSONEncoder(encoderConfig)
}

func parseZapLevel(level string) zapcore.Level {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		return zapcore.InfoLevel
	}
	return lvl
}

func zapFields(keysAndValues ...interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		key := fmt.Sprint(keysAndValues[i])
		if i+1 >= len(keysAndValues) {
			fields = append(fields, zap.Any(key, nil))
			continue
		}
		fields = append(fields, zap.Any(key, keysAndValues[i+1]))
	}
	return fields
}

func withTraceID(fields []zap.Field) []zap.Field {
	for _, field := range fields {
		if field.Key == TraceIDKey {
			return fields
		}
	}
	out := make([]zap.Field, 0, len(fields)+1)
	out = append(out, zap.String(TraceIDKey, ""))
	out = append(out, fields...)
	return out
}

func hasErrorField(fields []zap.Field) bool {
	for _, field := range fields {
		if field.Key == "error" || field.Key == "err" {
			return true
		}
	}
	return false
}
