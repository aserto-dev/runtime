package decision_log

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/open-policy-agent/opa/plugins"
	"github.com/rs/zerolog"
)

const PluginName = "best_decision_log"

type Config struct {
	Enabled bool `mapstructure:"enabled"`
}

type DecisionLogger struct {
	manager *plugins.Manager
	logger  zerolog.Logger
}

func newDecisionLogger(cfg *Config, manager *plugins.Manager) *DecisionLogger {
	var writer io.Writer

	if cfg.Enabled {
		logger := os.Stdout
		writer = logger
	} else {
		writer = io.Discard
	}

	return &DecisionLogger{
		logger:  zerolog.New(writer).With().Str("component", "decision-logger").Logger(),
		manager: manager,
	}
}

func (dl *DecisionLogger) Start(ctx context.Context) error {
	dl.logger.Debug().Msg("Start called")
	dl.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})

	return nil
}

func (dl *DecisionLogger) Stop(ctx context.Context) {
	dl.logger.Debug().Msg("Stop called")
	dl.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})
}

func (dl *DecisionLogger) Reconfigure(ctx context.Context, config interface{}) {
}

type Event struct {
	DecisionID string
	Timestamp  time.Time
}

func (dl *DecisionLogger) Log(ctx context.Context, event *Event) error {
	dl.logger.Log().
		Str("decision_id", event.DecisionID).
		Time("decision_time", event.Timestamp).
		Send()

	return nil
}

func Lookup(m *plugins.Manager) (*DecisionLogger, error) {
	p := m.Plugin(PluginName)
	if p == nil {
		return nil, errors.New("can't find decision logger")
	}
	dl, ok := p.(*DecisionLogger)
	if !ok {
		return nil, errors.New("decision logger is not a *DecisionLogger")
	}
	return dl, nil
}
