package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aserto-dev/runtime/logger"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/bundle"
	"github.com/open-policy-agent/opa/v1/loader"
	"github.com/open-policy-agent/opa/v1/metrics"
	"github.com/open-policy-agent/opa/v1/plugins"
	bundleplugin "github.com/open-policy-agent/opa/v1/plugins/bundle"
	"github.com/open-policy-agent/opa/v1/plugins/discovery"
	opaStatus "github.com/open-policy-agent/opa/v1/plugins/status"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/open-policy-agent/opa/v1/topdown/cache"
	"github.com/open-policy-agent/opa/v1/version"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Runtime manages the OPA runtime (plugins, store and info data).
type Runtime struct {
	Logger          *zerolog.Logger
	Config          *Config
	InterQueryCache cache.InterQueryCache

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
	bundlesCallbackRegistered   atomic.Bool
	discoveryCallbackRegistered atomic.Bool

	storage     storage.Store
	latestState atomic.Pointer[State]
	regoVersion ast.RegoVersion
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

// newOPARuntime creates a new OPA Runtime.
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
		regoVersion:  ast.RegoV0,
	}

	runtime.latestState.Store(&State{})

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

	if err := runtime.registerStatusPlugin([]string{"discovery"}); err != nil {
		return nil, nil, err
	}

	if cfg.LocalBundles.Watch {
		log.Info().Msg("Will start watching local bundles for changes")

		if err := runtime.startWatcher(ctx, cfg.LocalBundles.Paths, runtime.onReloadLogger); err != nil {
			log.Error().Err(err).Msg("unable to open watch")
			return nil, nil, errors.Wrap(err, "unable to open watch for local bundles")
		}
	}

	runtime.latestState.Store(runtime.status())

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

	// Cannot pass runtime.PluginsManager.Services() as the discovery service does not respond to POST on /status route.
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
	return r.latestState.Load()
}

func (r *Runtime) setLatestStatus(status *State) {
	r.latestState.Store(status)
}

func (r *Runtime) status() *State {
	result := &State{
		Ready:   true,
		Errors:  []error{},
		Bundles: []BundleState{},
	}

	r.pluginStates.Range(func(key, value interface{}) bool {
		pluginName, ok := key.(string)
		if !ok {
			return false
		}

		state, ok := value.(*pluginState)
		if !ok {
			return false
		}

		if !state.loaded {
			result.Ready = false
		}

		if state.err != nil {
			result.Errors = append(result.Errors, errors.Wrapf(state.err, "plugin '%s' encountered an error", pluginName))
		}

		return true
	})

	r.bundleStates.Range(func(key, value interface{}) bool {
		bundleID, ok := key.(string)
		if !ok {
			return false
		}

		state, ok := value.(*bundleState)
		if !ok {
			return false
		}

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

// newOPAPluginsManager creates a new OPA plugins.Manager.
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

	const maxParts int = 2

	for _, s := range os.Environ() {
		parts := strings.SplitN(s, "=", maxParts)
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
		plugins.WithParserOptions(ast.ParserOptions{RegoVersion: r.regoVersion}),
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

	// Note: this line is useless because the manager initializes the compiler
	// during init, and we don't have any control over it.
	// The compiler creates its own builtins array during its own init(), and
	// afterwards that cannot be changed anymore.
	// We have to improve this in order to have per runtime builtins.
	// manager.GetCompiler().WithBuiltins(r.compilerBuiltins).

	return manager, nil
}

// loadPaths reads data and policy from the given paths and returns a set of bundles
// if paths is not set, paths will be loaded from cfg.LocalBundles.Paths.
func (r *Runtime) loadPaths(paths []string) (map[string]*bundle.Bundle, error) {
	if len(paths) == 0 {
		paths = r.Config.LocalBundles.Paths
	}

	if r.Config.LocalBundles.LocalPolicyImage != "" {
		tarballpath, err := r.getPolicyTarballPath(r.Config.LocalBundles.LocalPolicyImage)
		if err != nil {
			r.Logger.Warn().Err(err).Msg("Could not load configured local policy image")
		}

		paths = append(paths, tarballpath)
	}

	result := make(map[string]*bundle.Bundle, len(paths))

	skipVerify := r.Config.LocalBundles.SkipVerification
	verificationConfig := r.Config.LocalBundles.VerificationConfig

	var err error

	for _, path := range paths {
		r.Logger.Info().Str("path", path).Msg("Loading local bundle")

		result[path], err = loader.NewFileLoader().
			WithBundleVerificationConfig(verificationConfig).
			WithSkipBundleVerification(skipVerify).
			AsBundle(path)
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

// Start - triggers plugin manager to start all plugins.
func (r *Runtime) Start(ctx context.Context) error {
	return r.pluginsManager.Start(ctx)
}

// Stop - triggers plugin manager to stop all plugins.
func (r *Runtime) Stop(ctx context.Context) {
	r.pluginsManager.Stop(ctx) // stop plugins always.
}

// GetPluginsManager returns the runtime plugin manager.
func (r *Runtime) GetPluginsManager() *plugins.Manager {
	return r.pluginsManager
}

func (r *Runtime) getPolicyTarballPath(policyImageRef string) (string, error) {
	if r.Config.LocalBundles.FileStoreRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.Wrap(err, "failed to determine user home directory")
		}

		r.Config.LocalBundles.FileStoreRoot = filepath.Join(home, ".policy")
	}

	// load index.json from root oci path
	type index struct {
		Version   int                  `json:"schemaVersion"`
		Manifests []ocispec.Descriptor `json:"manifests"`
	}

	indexPath := filepath.Join(r.Config.LocalBundles.FileStoreRoot, "policies-root", "index.json")

	time.Sleep(1 * time.Second) // wait until index.json is updated

	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return "", err
	}

	if len(indexBytes) == 0 {
		return "", errors.Errorf("empty index.json file")
	}

	var localIndex index
	if err := json.Unmarshal(indexBytes, &localIndex); err != nil {
		return "", err
	}

	// load manifest for policyImageRef
	var search ocispec.Descriptor

	for _, manifest := range localIndex.Manifests {
		if strings.Contains(manifest.Annotations[ocispec.AnnotationRefName], policyImageRef) &&
			manifest.MediaType == ocispec.MediaTypeImageLayerGzip {
			return filepath.Join(r.Config.LocalBundles.FileStoreRoot, "policies-root", "blobs", "sha256", manifest.Digest.Hex()), nil
		}

		if strings.Contains(manifest.Annotations[ocispec.AnnotationRefName], policyImageRef) &&
			manifest.MediaType == ocispec.MediaTypeImageManifest {
			search = manifest
			break
		}
	}

	if search.Digest == "" {
		return "", errors.Errorf("could not find policy image %s with a supported media type ('%s' or '%s')",
			policyImageRef, ocispec.MediaTypeImageManifest, ocispec.MediaTypeImageLayerGzip,
		)
	}

	manifestFile := filepath.Join(r.Config.LocalBundles.FileStoreRoot, "policies-root", "blobs", "sha256", search.Digest.Hex())

	manifestBytes, err := os.ReadFile(manifestFile)
	if err != nil {
		return "", err
	}

	var searchedManifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &searchedManifest); err != nil {
		return "", err
	}

	if len(searchedManifest.Layers) != 1 {
		return "", errors.New("unknown image type - incorrect number of layers")
	}

	tarballPath := filepath.Join(
		r.Config.LocalBundles.FileStoreRoot,
		"policies-root",
		"blobs",
		"sha256",
		searchedManifest.Layers[0].Digest.Hex(),
	)

	return tarballPath, nil
}

func (r *Runtime) WithRegoV1() *Runtime {
	r.regoVersion = ast.RegoV1
	return r
}
