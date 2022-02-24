package runtime

import (
	"context"
	"time"

	"github.com/aserto-dev/go-utils/cerr"
	"github.com/hashicorp/go-multierror"
	"github.com/open-policy-agent/opa/metrics"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/plugins/bundle"
	"github.com/open-policy-agent/opa/plugins/discovery"
	"github.com/pkg/errors"
)

const (
	bundleErrorCode     = "bundle_error"
	discoveryPluginName = "discovery"
	bundlePluginName    = "bundle"
)

type PluginDefinition struct {
	Name    string
	Factory plugins.Factory
}

// WaitForPlugins waits for all plugins to be ready
func (r *Runtime) WaitForPlugins(timeoutCtx context.Context, maxWaitTime time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(timeoutCtx, maxWaitTime)
	defer cancel()
	for {
		s := r.Status()
		if s.Ready {
			r.Logger.Info().Msg("runtime is ready")
			return nil
		}
		errs := s.Errors

		for i := range s.Bundles {
			errs = append(errs, s.Bundles[i].Errors...)
		}
		if len(errs) > 0 {
			return cerr.ErrBadRuntime.Err(multierror.Append(nil, errs...))
		}

		if timeoutCtx.Err() != nil {
			return cerr.ErrRuntimeLoading.Err(timeoutCtx.Err()).Msg("timeout while waiting for runtime to load")
		}

		time.Sleep(10 * time.Millisecond)
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
	if r.PluginsManager == nil {
		return false
	}

	pluginStates := r.PluginsManager.PluginStatus()
	for pluginName, status := range pluginStates {
		if status == nil || status.State == plugins.StateOK {
			continue
		}

		if pluginName == discoveryPluginName && r.Config.Config.Discovery == nil {
			continue
		}

		r.Logger.Trace().Str("state", string(status.State)).Str("plugin-name", pluginName).Msg("plugin not ready")
		return false
	}

	return true
}

//nolint TODO: This change would require upstream changes in OPA
func (r *Runtime) bundlesStatusCallback(status bundle.Status) {
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

	r.latestState = r.status()
}

//nolint // hugeParam - the status is heavy 200 bytes, upstream changes might be welcomed

func (r *Runtime) pluginStatusCallback(status map[string]*plugins.Status) {
	for n, s := range status {
		if n == bundlePluginName && !r.bundlesCallbackRegistered {
			plugin := r.PluginsManager.Plugin(bundlePluginName)
			if plugin != nil {
				bundlePlugin := plugin.(*bundle.Plugin)
				bundlePlugin.Register("aserto-error-recorder", r.bundlesStatusCallback)
				r.bundlesCallbackRegistered = true
			}
		}
		if n == discoveryPluginName && !r.discoveryCallbackRegistered {
			plugin := r.PluginsManager.Plugin(discoveryPluginName)
			if plugin != nil {
				discoveryPlugin := plugin.(*discovery.Discovery)
				discoveryPlugin.RegisterListener("aserto-error-recorder", r.bundlesStatusCallback)
				r.discoveryCallbackRegistered = true
			}
		}

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

	r.latestState = r.status()
}
