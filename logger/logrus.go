package logger

import (
	"io"

	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus" // nolint // we're only using logrus to make sure any packages using it still end up to zerolog
)

func AddLogrusHook(logger *zerolog.Logger) {
	logrusLogger := logger.With().Str("log-source", "logrus").Logger()
	logrus.AddHook(&logrusHook{logger: &logrusLogger})
	logrus.SetLevel(logrus.TraceLevel)
	logrus.SetOutput(io.Discard)

}

// logrusHook is a logrus hook that writes to zerolog.
type logrusHook struct {
	logger *zerolog.Logger
}

var _ logrus.Hook = &logrusHook{}

// Fire will be called when some logging function is called.
func (hook *logrusHook) Fire(entry *logrus.Entry) error {
	switch entry.Level {
	case logrus.PanicLevel:
		hook.logger.Panic().Fields(entry.Data).Msg(entry.Message)
	case logrus.FatalLevel:
		hook.logger.Fatal().Fields(entry.Data).Msg(entry.Message)
	case logrus.ErrorLevel:
		hook.logger.Error().Fields(entry.Data).Msg(entry.Message)
	case logrus.WarnLevel:
		hook.logger.Warn().Fields(entry.Data).Msg(entry.Message)
	case logrus.InfoLevel:
		hook.logger.Info().Fields(entry.Data).Msg(entry.Message)
	case logrus.DebugLevel:
		hook.logger.Debug().Fields(entry.Data).Msg(entry.Message)
	case logrus.TraceLevel:
		hook.logger.Trace().Fields(entry.Data).Msg(entry.Message)
	}

	return nil
}

// Levels defines on which log levels this hook would trigger.
func (hook *logrusHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
		logrus.TraceLevel,
	}
}
