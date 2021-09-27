package runtime

import (
	"context"

	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/rs/zerolog"
)

// asertoStore implements the OPA storage interface for the Aserto Runtime
type asertoStore struct {
	logger *zerolog.Logger
	cfg    *Config

	backend storage.Store
}

// newAsertoStore creates a new AsertoStore
func newAsertoStore(logger *zerolog.Logger, cfg *Config) *asertoStore {
	newLogger := logger.With().Str("source", "aserto-storage").Logger()
	return &asertoStore{
		logger:  &newLogger,
		cfg:     cfg,
		backend: inmem.New(),
	}
}

// NewTransaction is called to create a new transaction in the store.
func (s *asertoStore) NewTransaction(ctx context.Context, params ...storage.TransactionParams) (storage.Transaction, error) {
	s.logger.Trace().Msg("new-transaction")
	return s.backend.NewTransaction(ctx, params...)
}

// Read is called to fetch a document referred to by path.
func (s *asertoStore) Read(ctx context.Context, txn storage.Transaction, path storage.Path) (interface{}, error) {
	s.logger.Trace().Str("path", path.String()).Msg("read")
	return s.backend.Read(ctx, txn, path)
}

// Write is called to modify a document referred to by path.
func (s *asertoStore) Write(ctx context.Context, txn storage.Transaction, op storage.PatchOp, path storage.Path, value interface{}) error {
	s.logger.Trace().Str("path", path.String()).Msg("write")
	return s.backend.Write(ctx, txn, op, path, value)
}

// Commit is called to finish the transaction. If Commit returns an error, the
// transaction must be automatically aborted by the Store implementation.
func (s *asertoStore) Commit(ctx context.Context, txn storage.Transaction) error {
	s.logger.Trace().Msg("commit")
	return s.backend.Commit(ctx, txn)
}

// Abort is called to cancel the transaction.
func (s *asertoStore) Abort(ctx context.Context, txn storage.Transaction) {
	s.logger.Trace().Msg("abort")
	s.backend.Abort(ctx, txn)
}

// Register registers a trigger with the storage
func (s *asertoStore) Register(ctx context.Context, txn storage.Transaction, config storage.TriggerConfig) (storage.TriggerHandle, error) {
	s.logger.Trace().Msg("register")
	return s.backend.Register(ctx, txn, config)
}

// ListPolicies lists all policies
func (s *asertoStore) ListPolicies(ctx context.Context, txn storage.Transaction) ([]string, error) {
	s.logger.Trace().Msg("list-policies")
	return s.backend.ListPolicies(ctx, txn)
}

// GetPolicy gets a policy
func (s *asertoStore) GetPolicy(ctx context.Context, txn storage.Transaction, id string) ([]byte, error) {
	s.logger.Trace().Str("id", id).Msg("get-policy")
	return s.backend.GetPolicy(ctx, txn, id)
}

// UpsertPolicy creates a policy, or updates it if it already exists
func (s *asertoStore) UpsertPolicy(ctx context.Context, txn storage.Transaction, id string, bs []byte) error {
	s.logger.Trace().Str("id", id).Msg("upsert-policy")
	return s.backend.UpsertPolicy(ctx, txn, id, bs)
}

// DeletePolicy deletes a policy
func (s *asertoStore) DeletePolicy(ctx context.Context, txn storage.Transaction, id string) error {
	s.logger.Trace().Str("id", id).Msg("delete-policy")
	return s.backend.DeletePolicy(ctx, txn, id)
}
