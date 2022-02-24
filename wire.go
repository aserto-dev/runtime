//go:build wireinject
// +build wireinject

package runtime

import (
	"context"

	"github.com/google/wire"
	"github.com/rs/zerolog"
)

func NewRuntime(ctx context.Context, logger *zerolog.Logger, cfg *Config, opts ...Option) (*Runtime, func(), error) {
	wire.Build(wire.NewSet(
		newOPARuntime,
	))
	return &Runtime{}, func() {}, nil
}
