package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aserto-dev/go-utils/logger"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/bundle"
	"github.com/open-policy-agent/opa/loader"
	"github.com/open-policy-agent/opa/metrics"
	"github.com/open-policy-agent/opa/plugins"
	bundleplugin "github.com/open-policy-agent/opa/plugins/bundle"
	"github.com/open-policy-agent/opa/plugins/discovery"
	opaStatus "github.com/open-policy-agent/opa/plugins/status"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/topdown/cache"
	"github.com/open-policy-agent/opa/version"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Runtime manages the OPA runtime (plugins, store and info data)
type Runtime struct {
	Logger          *zerolog.Logger
	Config          *Config
	InterQueryCache cache.InterQueryCache
	Started         bool

	pluginsManager *plugins.Manager
	plugins        map[string]plugins.Factory

	builtins1        map[*rego.Function]rego.Builtin1
	builtins2        map[*rego.Function]rego.Builtin2
	builtins3        map[*rego.Function]rego.Builtin3
	builtins4        map[*rego.Function]rego.Builtin4
	builtinsDyn      map[*rego.Function]rego.BuiltinDyn
	builtins         []func(*rego.Rego)
	compilerBuiltins map[string]*ast.Builtin
	imports          []string

	pluginStates                *sync.Map
	bundleStates                *sync.Map
	bundlesCallbackRegistered   bool
	discoveryCallbackRegistered bool

	storage     storage.Store
	latestState *State
}

type BundleState struct {
	ID             string
	Revision       string
	LastDownload   time.Time
	LastActivation time.Time
	Errors         []error
}

type State struct {
	Ready   bool
	Errors  []error
	Bundles []BundleState
}

var builtinsLock sync.Mutex

// newOPARuntime creates a new OPA Runtime
func newOPARuntime(ctx context.Context, log *zerolog.Logger, cfg *Config, opts ...Option) (*Runtime, func(), error) {
	newLogger := log.With().Str("component", "runtime").Str("instance-id", cfg.InstanceID).Logger()

	runtime := &Runtime{
		Logger: &newLogger,
		Config: cfg,

		builtins1:        map[*rego.Function]rego.Builtin1{},
		builtins2:        map[*rego.Function]rego.Builtin2{},
		builtins3:        map[*rego.Function]rego.Builtin3{},
		builtins4:        map[*rego.Function]rego.Builtin4{},
		builtinsDyn:      map[*rego.Function]rego.BuiltinDyn{},
		builtins:         []func(*rego.Rego){},
		compilerBuiltins: map[string]*ast.Builtin{},

		pluginStates: &sync.Map{},
		bundleStates: &sync.Map{},
		plugins:      map[string]plugins.Factory{},
		latestState:  &State{},
	}

	for _, opt := range opts {
		opt(runtime)
	}

	if runtime.storage == nil {
		runtime.storage = inmem.New()
	}

	// We shouldn't register global builtins, these should be per runtime.
	// In order for that to work, the plugin manager has to allow us to tell the compiler
	// of our builtins.
	builtinsLock.Lock()
	defer builtinsLock.Unlock()
	for decl, impl := range runtime.builtins1 {
		log.Info().Str("name", decl.Name).Msg("registering builtin1")
		rego.RegisterBuiltin1(decl, impl)
	}

	for decl, impl := range runtime.builtins2 {
		log.Info().Str("name", decl.Name).Msg("registering builtin2")
		rego.RegisterBuiltin2(decl, impl)
	}

	for decl, impl := range runtime.builtins3 {
		log.Info().Str("name", decl.Name).Msg("registering builtin3")
		rego.RegisterBuiltin3(decl, impl)
	}

	for decl, impl := range runtime.builtins4 {
		log.Info().Str("name", decl.Name).Msg("registering builtin4")
		rego.RegisterBuiltin4(decl, impl)
	}

	for decl, impl := range runtime.builtinsDyn {
		log.Info().Str("name", decl.Name).Msg("registering builtinDyn")
		rego.RegisterBuiltinDyn(decl, impl)
	}

	var err error
	runtime.pluginsManager, err = runtime.newOPAPluginsManager(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to setup plugin manager")
	}

	runtime.InterQueryCache = cache.NewInterQueryCache(runtime.pluginsManager.InterQueryBuiltinCacheConfig())

	registeredPlugins := map[string]plugins.Factory{}

	for pluginName, factory := range runtime.plugins {
		log.Info().Str("plugin-name", pluginName).Msg("registering plugin")
		registeredPlugins[pluginName] = factory
	}

	m := metrics.New()
	disco, err := discovery.New(runtime.pluginsManager, discovery.Factories(registeredPlugins), discovery.Metrics(m))
	if err != nil {
		return nil, nil, errors.Wrap(err, "config error")
	}
	runtime.pluginsManager.Register("discovery", disco)
	err = runtime.registerStatusPlugin([]string{"discovery"})
	if err != nil {
		return nil, nil, err
	}

	if cfg.LocalBundles.Watch {
		log.Info().Msg("Will start watching local bundles for changes")
		err := runtime.startWatcher(ctx, cfg.LocalBundles.Paths, runtime.onReloadLogger)
		if err != nil {
			log.Error().Err(err).Msg("unable to open watch")
			return nil, nil, errors.Wrap(err, "unable to open watch for local bundles")
		}
	}

	runtime.latestState = runtime.status()

	return runtime,
		func() {
			runtime.Stop(ctx)
		},
		nil
}

func (r *Runtime) registerStatusPlugin(pluginNames []string) error {
	if !r.Config.Flags.EnableStatusPlugin {
		r.Logger.Debug().Msg("status plugin not registered")
		return nil
	}
	r.Logger.Debug().Msg("registering status plugin")
	rawconfig, err := r.Config.rawOPAConfig()
	if err != nil {
		return errors.Wrap(err, "raw config error")
	}

	// Cannot pass runtime.PluginsManager.Services() as the discovery service does not respond to POST on /status route
	statusConfig, err := opaStatus.NewConfigBuilder().WithBytes(rawconfig).
		WithServices([]string{""}).
		WithPlugins(pluginNames).Parse()
	if err != nil {
		return errors.Wrap(err, "failed to build status service config")
	}
	statusPlugin := opaStatus.New(statusConfig, r.pluginsManager)
	r.pluginsManager.Register("status", statusPlugin)
	return nil
}

func (r *Runtime) BuiltinRequirements() (json.RawMessage, error) {
	defs := fakeBuiltinDefs{}

	for f := range r.builtins1 {
		defs.Builtin1 = append(defs.Builtin1, fakeBuiltin1{
			Name: f.Name,
			Decl: *f.Decl,
		})
	}

	for f := range r.builtins2 {
		defs.Builtin2 = append(defs.Builtin2, fakeBuiltin2{
			Name: f.Name,
			Decl: *f.Decl,
		})
	}

	for f := range r.builtins3 {
		defs.Builtin3 = append(defs.Builtin3, fakeBuiltin3{
			Name: f.Name,
			Decl: *f.Decl,
		})
	}

	for f := range r.builtins4 {
		defs.Builtin4 = append(defs.Builtin4, fakeBuiltin4{
			Name: f.Name,
			Decl: *f.Decl,
		})
	}

	for f := range r.builtinsDyn {
		defs.BuiltinDyn = append(defs.BuiltinDyn, fakeBuiltinDyn{
			Name: f.Name,
			Decl: *f.Decl,
		})
	}

	jsonBytes, err := json.Marshal(defs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal builtin signatures into JSON")
	}

	return jsonBytes, nil
}

func (r *Runtime) Status() *State {
	return r.latestState
}

func (r *Runtime) status() *State {
	result := &State{
		Ready:   true,
		Errors:  []error{},
		Bundles: []BundleState{},
	}

	r.pluginStates.Range(func(key, value interface{}) bool {
		pluginName := key.(string)
		state := value.(*pluginState)
		if !state.loaded {
			result.Ready = false
		}

		if state.err != nil {
			result.Errors = append(result.Errors, errors.Wrapf(state.err, "plugin '%s' encountered an error", pluginName))
		}

		return true
	})

	r.bundleStates.Range(func(key, value interface{}) bool {
		bundleID := key.(string)
		state := value.(*bundleState)

		bs := BundleState{
			ID:             bundleID,
			Revision:       state.revision,
			LastDownload:   state.lastDownload,
			LastActivation: state.lastActivation,
			Errors:         state.errors,
		}

		if state.lastActivation.Equal(time.Time{}) {
			bs.Errors = append(
				bs.Errors,
				errors.New("bundle has never been activated"),
			)
		}

		result.Bundles = append(result.Bundles, bs)

		return true
	})

	result.Ready = r.pluginsLoaded()

	return result
}

// newOPAPluginsManager creates a new OPA plugins.Manager
func (r *Runtime) newOPAPluginsManager(ctx context.Context) (*plugins.Manager, error) {
	r.Logger.Info().Msg("creating OPA plugins manager")

	info := ast.NewObject()
	if r.Config != nil {
		v, err := ast.InterfaceToValue(r.Config.Config)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert config as an opa term")
		}

		info.Insert(ast.StringTerm("config"), ast.NewTerm(v))
	}

	env := ast.NewObject()

	r.Logger.Debug().Msg("loading process environment variables as rego terms")
	for _, s := range os.Environ() {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) == 1 {
			env.Insert(ast.StringTerm(parts[0]), ast.NullTerm())
		} else if len(parts) > 1 {
			env.Insert(ast.StringTerm(parts[0]), ast.StringTerm(parts[1]))
		}
	}

	info.Insert(ast.StringTerm("env"), ast.NewTerm(env))
	info.Insert(ast.StringTerm("version"), ast.StringTerm(version.Version))
	info.Insert(ast.StringTerm("commit"), ast.StringTerm(version.Vcs))

	loadedBundles, err := r.loadPaths([]string{})
	if err != nil {
		return nil, errors.Wrap(err, "local bundle load error")
	}

	rawConfig, err := r.Config.rawOPAConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal raw config")
	}

	manager, err := plugins.New(
		rawConfig,
		r.Config.InstanceID,
		r.storage,
		plugins.InitBundles(loadedBundles),
		plugins.Info(ast.NewTerm(info)),
		plugins.MaxErrors(r.Config.PluginsErrorLimit),
		plugins.GracefulShutdownPeriod(r.Config.GracefulShutdownPeriodSeconds),
		plugins.Logger(logger.NewOpaLogger(r.Logger)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize OPA plugins")
	}

	manager.RegisterPluginStatusListener("aserto-error-recorder", r.pluginStatusCallback)

	if err := manager.Init(ctx); err != nil {
		return nil, errors.Wrap(err, "initialization error")
	}

	// TODO: this line is useless because the manager initializes the compiler
	// during init, and we don't have any control over it.
	// The compiler creates its own builtins array during its own init(), and
	// afterwards that cannot be changed anymore.
	// We have to improve this in order to have per runtime builtins.
	// manager.GetCompiler().WithBuiltins(r.compilerBuiltins)

	return manager, nil
}

// loadPaths reads data and policy from the given paths and returns a set of bundles
// if paths is not set, paths will be loaded from cfg.LocalBundles.Paths
func (r *Runtime) loadPaths(paths []string) (map[string]*bundle.Bundle, error) {
	if len(paths) == 0 {
		paths = r.Config.LocalBundles.Paths
	}

	result := make(map[string]*bundle.Bundle, len(paths))

	skipVerify := r.Config.LocalBundles.SkipVerification
	verificationConfig := r.Config.LocalBundles.VerificationConfig

	var err error

	for _, path := range paths {
		r.Logger.Info().Str("path", path).Msg("Loading local bundle")
		result[path], err = loader.NewFileLoader().WithBundleVerificationConfig(verificationConfig).
			WithSkipBundleVerification(skipVerify).AsBundle(path)

		if err != nil {
			errorStatus := bundleplugin.Status{
				Name: path,
			}
			errorStatus.SetError(err)

			r.bundlesStatusCallback(errorStatus)

			return nil, errors.Wrapf(err, "load bundle from local path '%s'", path)
		}

		r.bundlesStatusCallback(
			bundleplugin.Status{
				Name:                     path,
				LastSuccessfulActivation: time.Now(),
				LastSuccessfulRequest:    time.Now(),
				LastSuccessfulDownload:   time.Now(),
				LastRequest:              time.Now(),
				ActiveRevision:           result[path].Manifest.Revision,
				Errors:                   []error{},
				Message:                  "local bundle loaded",
			})
	}

	return result, nil
}

// Start - triggers plugin manager to start all plugins
func (r *Runtime) Start(ctx context.Context) error {
	r.Started = true
	return r.pluginsManager.Start(ctx)
}

// Stop - triggers plugin manager to stop all plugins
func (r *Runtime) Stop(ctx context.Context) {
	if r.Started {
		r.pluginsManager.Stop(ctx)
		r.Started = false
	}
}

// GetPluginsManager returns the runtime plugin manager
func (r *Runtime) GetPluginsManager() *plugins.Manager {
	return r.pluginsManager
}
