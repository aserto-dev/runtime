package main

import (
	"fmt"

	runtime "github.com/aserto-dev/runtime"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/types"
	"github.com/pkg/errors"
)

type SigCmd struct {
	Verbosity int `       short:"v" type:"counter" help:"Use to increase output verbosity." default:"0"`
}

func (c *SigCmd) Run() error {
	ctx := setupLoggerAndContext(c.Verbosity)

	r, err := runtime.New(ctx, &runtime.Config{},
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

	def, err := r.BuiltinRequirements()
	if err != nil {
		return errors.Wrap(err, "failed to calculate builtin requirements")
	}

	fmt.Println(string(def))

	return nil
}
