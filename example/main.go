package main

import (
	"context"
	"os"

	"github.com/alecthomas/kong"
	"github.com/rs/zerolog"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

const (
	verbosityError = 0
	verbosityInfo  = 1
	verbosityDebug = 2
	verbosityTrace = 3
)

type Verdict struct {
	Query  QueryCmd  `cmd:"" help:"Run a query against a policy."`
	QueryX QueryXCmd `cmd:"" help:"Run a query against a policy using an extended runtime."`
	Build  BuildCmd  `cmd:"" help:"Build a policy into a bundle."`
	Sig    SigCmd    `cmd:"" help:"Prints builtin requirements."`
}

func main() {
	verdict := &Verdict{}
	ctx := kong.Parse(verdict)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}

func setupLoggerAndContext(verbosity int) (context.Context, *zerolog.Logger) { //nolint:ireturn
	ctx := signals.SetupSignalHandler()
	logger := zerolog.New(os.Stdout)

	switch verbosity {
	case verbosityError:
		logger = logger.Level(zerolog.ErrorLevel)
	case verbosityInfo:
		logger = logger.Level(zerolog.InfoLevel)
	case verbosityDebug:
		logger = logger.Level(zerolog.DebugLevel)
	default:
		logger = logger.Level(zerolog.TraceLevel)
	}

	return ctx, &logger
}
