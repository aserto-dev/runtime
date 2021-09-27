package runtime

import (
	"context"
	"time"

	"github.com/open-policy-agent/opa/metrics"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/plugins/bundle"
	"github.com/pkg/errors"
)

const (
	bundleErrorCode = "bundle_error"
)

type PluginDefinition struct {
	Name    string
	Factory plugins.Factory
}

// WaitForPlugins waits for all plugins to be ready
func (r *Runtime) WaitForPlugins(ctx context.Context, maxWaitTime time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, maxWaitTime)
	defer cancel()

	for {
		s := r.Status()
		if s.Ready {
			r.Logger.Info().Msg("runtime is ready")
			return nil
		}

		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "waiting for plugins")
		}

		time.Sleep(10 * time.Millisecond) //nolint:gomnd
	}
}

type bundleState struct {
	revision       string
	errors         []error
	message        string
	metrics        metrics.Metrics
	lastActivation time.Time
	lastDownload   time.Time
}

type pluginState struct {
	err    error
	loaded bool
}

// pluginsLoaded returns true if all plugins have been loaded
func (r *Runtime) pluginsLoaded() bool {
	pluginStates := r.PluginsManager.PluginStatus()
	for pluginName, status := range pluginStates {
		if status == nil || status.State == plugins.StateOK {
			continue
		}

		r.Logger.Trace().Str("state", string(status.State)).Str("plugin-name", pluginName).Msg("plugin not ready")
		return false
	}

	return true
}

func (r *Runtime) bundlesErrorRecorder(status bundle.Status) {

	errs := status.Errors
	if status.Code == bundleErrorCode {
		errs = append(errs, errors.Errorf("bundle error: %s", status.Message))
	}

	r.bundleStates.Store(status.Name, &bundleState{
		revision:       status.ActiveRevision,
		errors:         errs,
		message:        status.Message,
		metrics:        status.Metrics,
		lastActivation: status.LastSuccessfulActivation,
		lastDownload:   status.LastSuccessfulDownload,
	})
}

func (r *Runtime) errorRecorder(status map[string]*plugins.Status) {
	for n, s := range status {
		if s == nil {
			continue
		}
		switch s.State {
		case plugins.StateErr:
			r.Logger.Trace().Str("runtime-id", r.PluginsManager.ID).Str("plugin", n).Msg("plugin in error state")
			r.pluginStates.Store(n, &pluginState{err: errors.New("there was an error loading the plugin"), loaded: false})
		case plugins.StateNotReady:
			r.Logger.Trace().Str("runtime-id", r.PluginsManager.ID).Str("plugin", n).Msg("plugin not ready")
			r.pluginStates.Store(n, &pluginState{loaded: false})
		case plugins.StateOK:
			r.Logger.Trace().Str("runtime-id", r.PluginsManager.ID).Str("plugin", n).Msg("plugin ready")
			r.pluginStates.Store(n, &pluginState{loaded: true})
		}
	}
}
