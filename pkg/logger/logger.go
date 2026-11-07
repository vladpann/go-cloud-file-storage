package logger

import (
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
}

type Logger struct {
	logger *zerolog.Logger
	lg     *lumberjack.Logger
}

var _ Interface = (*Logger)(nil)

func New(level string, folder string) (*Logger, error) {
	l := parseLevel(level)

	if err := os.MkdirAll(folder, 0755); err != nil {
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
