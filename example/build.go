package main

import (
	runtime "github.com/aserto-dev/runtime"
	"github.com/pkg/errors"
)

type BuildCmd struct {
	Path      []string `arg:"" short:"b" type:"string"  help:"Path to local policies."           default:"."`
	Output    string   `       short:"o" type:"path"    help:"Output path."                      default:"./bundle.tar.gz"`
	Verbosity int      `       short:"v" type:"counter" help:"Use to increase output verbosity." default:"0"`
}

func (c *BuildCmd) Run() error {
	ctx := setupLoggerAndContext(c.Verbosity)

	r, err := runtime.New(ctx, &runtime.Config{})
	if err != nil {
		return errors.Wrap(err, "failed to create runtime")
	}

	return r.Build(&runtime.BuildParams{
		OutputFile: c.Output,
	}, c.Path)
}
