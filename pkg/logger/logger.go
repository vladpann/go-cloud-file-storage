package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Interface interface {
	Debug(message any, args ...any)
	Info(message string, args ...any)
	Warn(message string, args ...any)
	Error(message any, args ...any)
	Fatal(message any, args ...any)

	// Context-aware logging methods
	DebugContext(ctx context.Context, message any, args ...any)
	InfoContext(ctx context.Context, message string, args ...any)
	WarnContext(ctx context.Context, message string, args ...any)
	ErrorContext(ctx context.Context, message any, args ...any)

	// GetZerolog returns the underlying zerolog.Logger for advanced use
	GetZerolog() *zerolog.Logger
}

type Logger struct {
	logger *zerolog.Logger
	lg     *lumberjack.Logger
}

var _ Interface = (*Logger)(nil)

func New(level, folder string) (*Logger, error) {
	l := parseLevel(level)

	if err := os.MkdirAll(folder, 0o755); err != nil {
		return nil, fmt.Errorf("logger - New - os.MkfirAll: %w", err)
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05.000000")
	lj := &lumberjack.Logger{
		Filename:   filepath.Join(folder, fmt.Sprintf("%s.log", timestamp)),
		MaxSize:    50,
		MaxAge:     1,
		MaxBackups: 0,
		Compress:   false,
		LocalTime:  true,
	}

	w := io.MultiWriter(os.Stdout, lj)

	skipFrameCount := 3
	logger := zerolog.
		New(w).
		With().
		Timestamp().
		CallerWithSkipFrameCount(zerolog.CallerSkipFrameCount + skipFrameCount).
		Logger().
		Level(l)

	// Set global logger for zerolog.Ctx to use
	zerolog.DefaultContextLogger = &logger

	return &Logger{
		logger: &logger,
		lg:     lj,
	}, nil
}

func (l *Logger) Debug(message any, args ...any) {
	l.msg(zerolog.DebugLevel, message, args...)
}

func (l *Logger) Info(message string, args ...any) {
	l.log(zerolog.InfoLevel, message, args...)
}

func (l *Logger) Warn(message string, args ...any) {
	l.log(zerolog.WarnLevel, message, args...)
}

func (l *Logger) Error(message any, args ...any) {
	l.msg(zerolog.ErrorLevel, message, args...)
}

func (l *Logger) Fatal(message any, args ...any) {
	l.msg(zerolog.FatalLevel, message, args...)

	os.Exit(1)
}

func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "error":
		return zerolog.ErrorLevel
	case "warn":
		return zerolog.WarnLevel
	case "info":
		return zerolog.InfoLevel
	case "debug":
		return zerolog.DebugLevel
	default:
		return zerolog.InfoLevel
	}
}

func (l *Logger) log(level zerolog.Level, message string, args ...any) {
	if len(args) == 0 {
		l.logger.WithLevel(level).Msg(message)
	} else {
		l.logger.WithLevel(level).Msgf(message, args...)
	}
}

func (l *Logger) msg(level zerolog.Level, message any, args ...any) {
	switch msg := message.(type) {
	case error:
		l.log(level, msg.Error(), args...)
	case string:
		l.log(level, msg, args...)
	default:
		l.log(level, fmt.Sprintf("%s message %v has unknown type %v", level, message, msg), args...)
	}
}

func (l *Logger) Close() {
	l.lg.Close()
}

// GetZerolog returns the underlying zerolog.Logger
func (l *Logger) GetZerolog() *zerolog.Logger {
	return l.logger
}

// Context-aware logging methods that use zerolog.Ctx to get logger from context

func (l *Logger) DebugContext(ctx context.Context, message any, args ...any) {
	logger := zerolog.Ctx(ctx)
	if logger.GetLevel() == zerolog.Disabled || logger == zerolog.DefaultContextLogger {
		logger = l.logger
	}
	l.msgWithLogger(logger, zerolog.DebugLevel, message, args...)
}

func (l *Logger) InfoContext(ctx context.Context, message string, args ...any) {
	logger := zerolog.Ctx(ctx)
	if logger.GetLevel() == zerolog.Disabled || logger == zerolog.DefaultContextLogger {
		logger = l.logger
	}
	l.logWithLogger(logger, zerolog.InfoLevel, message, args...)
}

func (l *Logger) WarnContext(ctx context.Context, message string, args ...any) {
	logger := zerolog.Ctx(ctx)
	if logger.GetLevel() == zerolog.Disabled || logger == zerolog.DefaultContextLogger {
		logger = l.logger
	}
	l.logWithLogger(logger, zerolog.WarnLevel, message, args...)
}

func (l *Logger) ErrorContext(ctx context.Context, message any, args ...any) {
	logger := zerolog.Ctx(ctx)
	if logger.GetLevel() == zerolog.Disabled || logger == zerolog.DefaultContextLogger {
		logger = l.logger
	}
	l.msgWithLogger(logger, zerolog.ErrorLevel, message, args...)
}

func (l *Logger) logWithLogger(logger *zerolog.Logger, level zerolog.Level, message string, args ...any) {
	if len(args) == 0 {
		logger.WithLevel(level).Msg(message)
	} else {
		logger.WithLevel(level).Msgf(message, args...)
	}
}

func (l *Logger) msgWithLogger(logger *zerolog.Logger, level zerolog.Level, message any, args ...any) {
	switch msg := message.(type) {
	case error:
		l.logWithLogger(logger, level, msg.Error(), args...)
	case string:
		l.logWithLogger(logger, level, msg, args...)
	default:
		l.logWithLogger(logger, level, fmt.Sprintf("%s message %v has unknown type %v", level, message, msg), args...)
	}
}
