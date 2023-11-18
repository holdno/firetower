package log

import (
	"os"
	"runtime"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Name         string
	Level        zapcore.Level
	File         string
	RotateConfig *RotateConfig
	Wrappers     []func(*zap.Logger) *zap.Logger
}

type RotateConfig struct {
	Compress   bool
	MaxAge     int
	MaxSize    int
	MaxBackups int
}

// New is a function to create new zap logger
// this logger implement the Logger interface
func New(cfg Config) *zap.Logger {
	var writer zapcore.WriteSyncer
	if cfg.File != "" {
		if cfg.RotateConfig == nil {
			cfg.RotateConfig = &RotateConfig{
				Compress:   true,
				MaxAge:     24,
				MaxSize:    100,
				MaxBackups: 1,
			}
		}
		writer = zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.RotateConfig.MaxSize,
			MaxBackups: cfg.RotateConfig.MaxBackups,
			MaxAge:     cfg.RotateConfig.MaxAge,
			Compress:   cfg.RotateConfig.Compress,
		})
	} else {
		writer = zapcore.AddSync(os.Stdout)

	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "name",
		MessageKey:     "msg",
		StacktraceKey:  "stack",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeName:     zapcore.FullNameEncoder,
		EncodeCaller:   zapcore.FullCallerEncoder,
	}

	// 设置日志级别
	atomicLevel := zap.NewAtomicLevel()
	atomicLevel.SetLevel(cfg.Level)

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		writer,
		atomicLevel,
	)

	// init fields
	if cfg.Name == "" {
		cfg.Name = "firetower"
	}
	initfields := zap.Fields(zap.String("service", cfg.Name), zap.String("runtime", runtime.Version()))
	logger := zap.New(core, zap.AddStacktrace(zapcore.PanicLevel), initfields)

	// wrap logger
	for _, wrapper := range cfg.Wrappers {
		logger = wrapper(logger)
	}

	return logger
}
