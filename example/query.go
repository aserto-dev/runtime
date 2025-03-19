package main

import (
	"encoding/json"
	"fmt"
	"os"

	runtime "github.com/aserto-dev/runtime"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

type QueryCmd struct {
	Policy    string `arg:"" short:"b" type:"path"    help:"Path to the policy bundle."        default:"./bundle.tar.gz"`
	Query     string `       short:"q" type:"string"  help:"Query to run."                     default:"x = data"`
	Input     string `       short:"i" type:"string"  help:"Input to the query, as JSON."      default:"{}"`
	Verbosity int    `       short:"v" type:"counter" help:"Use to increase output verbosity." default:"0"`
}

func (c *QueryCmd) Run() error {
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
	})
	if err != nil {
		return errors.Wrap(err, "failed to create runtime")
	}
	defer cleanup()

	input := map[string]interface{}{}
	if err := json.Unmarshal([]byte(c.Input), &input); err != nil {
		return errors.Wrap(err, "invalid input parameter")
	}

	result, err := r.Query(ctx, c.Query, input, true, false, false, "")
	if err != nil {
		return errors.Wrap(err, "query error")
	}

	out, err := json.MarshalIndent(result.Result, "", "  ")
	if err != nil {
		return errors.Wrap(err, "can't marshal output json")
	}

	fmt.Printf("%s\n", out)
	return nil
}
