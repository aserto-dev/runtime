package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	runtime "github.com/aserto-dev/runtime"
	"github.com/aserto-dev/verdict/plugins/decision_log"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

type QueryXCmd struct {
	Policy    string `arg:"" short:"b" type:"path"    help:"Path to the policy bundle."        default:"./bundle.tar.gz"`
	Query     string `       short:"q" type:"string"  help:"Query to run."                     default:"x = data"`
	Input     string `       short:"i" type:"string"  help:"Input to the query, as JSON."      default:"{}"`
	Verbosity int    `       short:"v" type:"counter" help:"Use to increase output verbosity." default:"0"`
}

func (c *QueryXCmd) Run() error {
	ctx := signals.SetupSignalHandler()
	logger := zerolog.New(os.Stdout)

	switch c.Verbosity {
	case 0:
		logger = logger.Level(zerolog.ErrorLevel)
	case 1:
		logger = logger.Level(zerolog.InfoLevel)
	case 2:
		logger = logger.Level(zerolog.DebugLevel)
	default:
		logger = logger.Level(zerolog.TraceLevel)
	}

	r, cleanup, err := runtime.NewRuntime(ctx, &logger, &runtime.Config{
		LocalBundles: runtime.LocalBundlesConfig{
			Paths: []string{c.Policy},
		},
		Config: runtime.OPAConfig{
			Plugins: map[string]interface{}{
				decision_log.PluginName: decision_log.Config{
					Enabled: true,
				},
			},
		},
	},
		runtime.WithPlugin(decision_log.PluginName, decision_log.NewPluginFactory()),
		runtime.WithBuiltin1(&rego.Function{
			Name:    "hello",
			Memoize: false,
			Decl:    types.NewFunction(types.Args(types.S), types.S),
		}, func(bctx rego.BuiltinContext, name *ast.Term) (*ast.Term, error) {
			strName := ""
			err := ast.As(name.Value, &strName)
			if err != nil {
				return nil, errors.Wrap(err, "name parameter is not a string")
			}

			if strName == "there" {
				return ast.StringTerm("general kenobi"), nil
			}
			return nil, nil
		}),
	)

	if err != nil {
		return errors.Wrap(err, "failed to create runtime")
	}
	defer cleanup()

	err = r.PluginsManager.Start(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to start plugin manager")
	}

	err = r.WaitForPlugins(ctx, time.Second*5)
	if err != nil {
		return errors.Wrap(err, "failed to create runtime")
	}

	input := map[string]interface{}{}
	err = json.Unmarshal([]byte(c.Input), &input)
	if err != nil {
		return errors.Wrap(err, "invalid input parameter")
	}

	result, err := r.Query(ctx, c.Query, input, true, false, false, "")
	if err != nil {
		return errors.Wrap(err, "query error")
	}

	decisionLogger, err := decision_log.Lookup(r.PluginsManager)
	if err != nil {
		return errors.Wrap(err, "decision logger lookup failed")
	}

	err = decisionLogger.Log(ctx, &decision_log.Event{
		DecisionID: result.DecisionID,
		Timestamp:  time.Now().UTC(),
	})
	if err != nil {
		return errors.Wrap(err, "failed to log decision")
	}

	out, err := json.MarshalIndent(result.Result, "", "  ")
	if err != nil {
		return errors.Wrap(err, "can't marshal output json")
	}

	fmt.Printf("%s\n", out)
	return nil
}
