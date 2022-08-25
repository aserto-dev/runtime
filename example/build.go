package main

import (
	"os"

	runtime "github.com/aserto-dev/runtime"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

type BuildCmd struct {
	Path      []string `arg:"" short:"b" type:"string"  help:"Path to local policies."           default:"."`
	Output    string   `       short:"o" type:"path"    help:"Output path."                      default:"./bundle.tar.gz"`
	Verbosity int      `       short:"v" type:"counter" help:"Use to increase output verbosity." default:"0"`
}

func (c *BuildCmd) Run() error {
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

	r, cleanup, err := runtime.NewRuntime(ctx, &logger, &runtime.Config{})

	if err != nil {
		return errors.Wrap(err, "failed to create runtime")
	}
	defer cleanup()

	return r.Build(&runtime.BuildParams{
		OutputFile: c.Output,
	}, c.Path)
}
