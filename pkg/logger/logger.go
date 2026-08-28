package logger

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
)

//go:generate mockgen -destination=../mocks/mock_logger.go -package=pkgmocks github.com/Notifuse/notifuse/pkg/logger Logger

type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Fatal(msg string)
	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
}

type zerologLogger struct {
	logger zerolog.Logger
}

func NewLogger() Logger {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	return &zerologLogger{
		logger: logger,
	}
}

// NewLoggerWithLevel creates a new logger with the specified log level
func NewLoggerWithLevel(level string) Logger {
	// Set the global log level based on the provided level
	switch strings.ToLower(level) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn", "warning":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case "panic":
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	case "disabled", "off":
		zerolog.SetGlobalLevel(zerolog.Disabled)
	default:
		// Default to info level if unknown level is provided
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	return &zerologLogger{
		logger: logger,
	}
}

func (l *zerologLogger) Debug(msg string) {
	l.logger.Debug().Msg(msg)
}

func (l *zerologLogger) Info(msg string) {
	l.logger.Info().Msg(msg)
}

func (l *zerologLogger) Warn(msg string) {
	l.logger.Warn().Msg(msg)
}

func (l *zerologLogger) Error(msg string) {
	l.logger.Error().Msg(msg)
}

func (l *zerologLogger) Fatal(msg string) {
	l.logger.Fatal().Msg(msg)
}

func (l *zerologLogger) WithField(key string, value interface{}) Logger {
	return &zerologLogger{
		logger: l.logger.With().Interface(key, value).Logger(),
	}
}

func (l *zerologLogger) WithFields(fields map[string]interface{}) Logger {
	// Build a new logger with the provided fields without mutating the receiver
	ctx := l.logger.With()
	for key, value := range fields {
		ctx = ctx.Interface(key, value)
	}
	newLogger := ctx.Logger()
	return &zerologLogger{logger: newLogger}
}
