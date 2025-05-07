package main

import (
	"encoding/json"
	"fmt"
	"time"

	runtime "github.com/aserto-dev/runtime"
	"github.com/aserto-dev/runtime/example/plugins/decision_log"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/types"
	"github.com/pkg/errors"
)

const pluginReadyTimeout = 5 * time.Second

type QueryXCmd struct {
	Policy    string `arg:"" short:"b" type:"path"    help:"Path to the policy bundle."        default:"./bundle.tar.gz"`
	Query     string `       short:"q" type:"string"  help:"Query to run."                     default:"x = data"`
	Input     string `       short:"i" type:"string"  help:"Input to the query, as JSON."      default:"{}"`
	Verbosity int    `       short:"v" type:"counter" help:"Use to increase output verbosity." default:"0"`
}

func (c *QueryXCmd) Run() error {
	ctx, logger := setupLoggerAndContext(c.Verbosity)

	r, err := runtime.NewRuntime(ctx, logger, &runtime.Config{
		LocalBundles: runtime.LocalBundlesConfig{
			Paths: []string{c.Policy},
		},
		Config: runtime.OPAConfig{
			Plugins: map[string]any{
				decision_log.PluginName: decision_log.Config{
					Enabled: true,
				},
			},
		},
	},
		runtime.WithPlugin(decision_log.PluginName, decision_log.NewPluginFactory()),
		runtime.WithBuiltin1(
			&rego.Function{
				Name:    "hello",
				Memoize: false,
				Decl:    types.NewFunction(types.Args(types.S), types.S),
			},
			func(bctx rego.BuiltinContext, name *ast.Term) (*ast.Term, error) {
				strName := ""

				if err := ast.As(name.Value, &strName); err != nil {
					return nil, errors.Wrap(err, "name parameter is not a string")
				}

				if strName == "there" {
					return ast.StringTerm("general kenobi"), nil
				}

				return &ast.Term{}, nil
			},
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create runtime")
	}

	if err := r.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start plugin manager")
	}

	if err := r.WaitForPlugins(ctx, pluginReadyTimeout); err != nil {
		return errors.Wrap(err, "failed to create runtime")
	}

	input := map[string]any{}
	if err := json.Unmarshal([]byte(c.Input), &input); err != nil {
		return errors.Wrap(err, "invalid input parameter")
	}

	result, err := r.Query(ctx, c.Query, input, true, false, false, "")
	if err != nil {
		return errors.Wrap(err, "query error")
	}

	decisionLogger, err := decision_log.Lookup(r.GetPluginsManager())
	if err != nil {
		return errors.Wrap(err, "decision logger lookup failed")
	}

	if err := decisionLogger.Log(ctx, &decision_log.Event{
		DecisionID: result.DecisionID,
		Timestamp:  time.Now().UTC(),
	}); err != nil {
		return errors.Wrap(err, "failed to log decision")
	}

	out, err := json.MarshalIndent(result.Result, "", "  ")
	if err != nil {
		return errors.Wrap(err, "can't marshal output json")
	}

	fmt.Printf("%s\n", out)

	return nil
}
