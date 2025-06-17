package main

import (
	"encoding/json"
	"fmt"

	runtime "github.com/aserto-dev/runtime"
	"github.com/pkg/errors"
)

type QueryCmd struct {
	Policy    string `arg:"" short:"b" type:"path"    help:"Path to the policy bundle."        default:"./bundle.tar.gz"`
	Query     string `       short:"q" type:"string"  help:"Query to run."                     default:"x = data"`
	Input     string `       short:"i" type:"string"  help:"Input to the query, as JSON."      default:"{}"`
	Verbosity int    `       short:"v" type:"counter" help:"Use to increase output verbosity." default:"0"`
}

func (c *QueryCmd) Run() error {
	ctx, logger := setupLoggerAndContext(c.Verbosity)

	r, err := runtime.New(ctx, logger, &runtime.Config{
		LocalBundles: runtime.LocalBundlesConfig{
			Paths: []string{c.Policy},
		},
	})
	if err != nil {
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

	out, err := json.MarshalIndent(result.Result, "", "  ")
	if err != nil {
		return errors.Wrap(err, "can't marshal output json")
	}

	fmt.Printf("%s\n", out)

	return nil
}
