package decision_log

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const PluginName = "best_decision_log"

var errLookup = errors.New("lookup error")

type Config struct {
	Enabled bool `json:"enabled"`
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

func (dl *DecisionLogger) Reconfigure(ctx context.Context, config any) {
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
		return nil, errors.Wrap(errLookup, "can't find decision logger")
	}

	dl, ok := p.(*DecisionLogger)
	if !ok {
		return nil, errors.Wrap(errLookup, "decision logger is not a *DecisionLogger")
	}

	return dl, nil
}
