package logger

import (
	"sync"

	"github.com/open-policy-agent/opa/v1/logging"
	"github.com/rs/zerolog"
)

type OpaLogger struct {
	logger    *zerolog.Logger
	fields    map[string]interface{}
	levelLock sync.Mutex
}

var _ logging.Logger = &OpaLogger{}

func NewOpaLogger(logger *zerolog.Logger) *OpaLogger {
	return &OpaLogger{logger: logger}
}

func (l *OpaLogger) Debug(fmt string, a ...interface{}) {
	l.logger.Debug().Msgf(fmt, a...)
}

func (l *OpaLogger) Info(fmt string, a ...interface{}) {
	l.logger.Info().Msgf(fmt, a...)
}

func (l *OpaLogger) Error(fmt string, a ...interface{}) {
	l.logger.Error().Msgf(fmt, a...)
}

func (l *OpaLogger) Warn(fmt string, a ...interface{}) {
	l.logger.Warn().Msgf(fmt, a...)
}

func (l *OpaLogger) WithFields(fields map[string]interface{}) logging.Logger {
	newLogger := l.logger.With().Fields(fields).Logger()
	logger := NewOpaLogger(&newLogger)
	logger.fields = make(map[string]interface{})

	for k, v := range l.fields {
		logger.fields[k] = v
	}

	for k, v := range fields {
		logger.fields[k] = v
	}

	return logger
}

func (l *OpaLogger) GetFields() map[string]interface{} {
	return l.fields
}

func (l *OpaLogger) GetLevel() logging.Level {
	switch l.logger.GetLevel() { //nolint:exhaustive
	case zerolog.DebugLevel:
		return logging.Debug
	case zerolog.InfoLevel:
		return logging.Info
	case zerolog.WarnLevel:
		return logging.Warn
	case zerolog.ErrorLevel:
		return logging.Error
	default:
		return logging.Error
	}
}

func (l *OpaLogger) SetLevel(level logging.Level) {
	zLevel := zerolog.DebugLevel

	switch level {
	case logging.Debug:
		zLevel = zerolog.DebugLevel
	case logging.Info:
		zLevel = zerolog.InfoLevel
	case logging.Warn:
		zLevel = zerolog.WarnLevel
	case logging.Error:
		zLevel = zerolog.ErrorLevel
	}

	l.levelLock.Lock()
	defer l.levelLock.Unlock()

	newLogger := l.logger.Level(zLevel)
	l.logger = &newLogger
}
