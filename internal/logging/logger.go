package logging

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New() (*zap.Logger, error) {
	var cfg zap.Config
	cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	cfg.Development = false
	cfg.Encoding = "json"
	cfg.OutputPaths = []string{"stderr"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	cfg.EncoderConfig = zap.NewProductionEncoderConfig()
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.NameKey = "logger"
	cfg.EncoderConfig.MessageKey = "msg"
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.StacktraceKey = "stacktrace"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	return cfg.Build(zap.AddCaller(), zap.AddCallerSkip(1))
}

func WithComponent(logger *zap.Logger, component string) *zap.Logger {
	if logger == nil {
		return zap.NewNop()
	}

	component = strings.TrimSpace(component)
	if component == "" {
		return logger
	}

	return logger.Named(component)
}
