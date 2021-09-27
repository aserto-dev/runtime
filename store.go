package runtime

import (
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/rs/zerolog"
)

// newOPAStore creates a new OPA storage store
func newOPAStore(logger *zerolog.Logger, cfg *Config) storage.Store {
	logger.Debug().Msg("creating new aserto opa data store")

	switch cfg.Store {
	case "inmem":
		return inmem.New()
	case "aserto":
		return newAsertoStore(logger, cfg)
	default:
		return inmem.New()
	}
}
