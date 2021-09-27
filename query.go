package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/metrics"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/server/types"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/open-policy-agent/opa/topdown/lineage"
	"github.com/pkg/errors"
)

// map of unsafe builtins
var unsafeBuiltinsMap = map[string]struct{}{ast.HTTPSend.Name: {}}

// Result contains the results of a Query execution
type Result struct {
	Result      rego.ResultSet
	Metrics     map[string]interface{}
	Explanation types.TraceV1
	DecisionID  string
}

// Query executes a REGO query against the Aserto OPA Runtime
// explain can be "notes", "full" or "off"
func (r *Runtime) Query(ctx context.Context, qStr string, input map[string]interface{}, pretty, includeMetrics, includeInstrumentation bool, explain types.ExplainModeV1) (*Result, error) {
	m := metrics.New()

	decisionID := uuid.New().String()

	parsedQuery, err := validateQuery(qStr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate query")
	}

	txn, err := r.Store.NewTransaction(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new OPA store transaction")
	}

	defer r.Store.Abort(ctx, txn)

	results, err := r.execQuery(ctx, txn, decisionID, parsedQuery, input, m, explain, includeMetrics, includeInstrumentation, pretty)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}

	return results, nil
}

func validateQuery(query string) (ast.Body, error) {
	var body ast.Body
	body, err := ast.ParseBody(query)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (r *Runtime) execQuery(ctx context.Context, txn storage.Transaction, decisionID string, parsedQuery ast.Body, input map[string]interface{}, m metrics.Metrics, explainMode types.ExplainModeV1, includeMetrics, includeInstrumentation, pretty bool) (*Result, error) {

	var buf *topdown.BufferTracer
	if explainMode != types.ExplainOffV1 {
		buf = topdown.NewBufferTracer()
	}

	opts := r.builtins

	compiler := r.PluginsManager.GetCompiler()

	opts = append(opts,
		rego.Store(r.Store),
		rego.Transaction(txn),
		rego.Compiler(compiler),
		rego.ParsedQuery(parsedQuery),
		rego.Metrics(m),
		rego.Instrument(includeInstrumentation),
		rego.QueryTracer(buf),
		rego.Trace(true),
		rego.Runtime(r.PluginsManager.Info),
		rego.UnsafeBuiltins(unsafeBuiltinsMap),
		rego.InterQueryBuiltinCache(r.InterQueryCache),
		rego.Input(input),
	)

	for _, r := range r.PluginsManager.GetWasmResolvers() {
		for _, entrypoint := range r.Entrypoints() {
			opts = append(opts, rego.Resolver(entrypoint, r))
		}
	}

	regoQuery := rego.New(opts...)

	output, err := regoQuery.Eval(ctx)
	if err != nil {
		r.Logger.Warn().
			Err(err).Str("decisionID", decisionID).
			Str("query", parsedQuery.String()).
			Interface("input", input).
			Msg("error evaluating query")

		return nil, errors.Wrap(err, "failed to evaluate rego query")
	}

	results := &Result{
		Result:     output,
		DecisionID: decisionID,
	}

	if includeMetrics || includeInstrumentation {
		results.Metrics = m.All()
	}

	if explainMode != types.ExplainOffV1 {
		results.Explanation = r.getExplainResponse(explainMode, *buf, pretty)
	}

	r.Logger.Debug().
		Err(err).Str("decisionID", decisionID).
		Str("query", parsedQuery.String()).
		Interface("input", input).
		Msg("query evaluated")

	return results, err
}

func (r *Runtime) getExplainResponse(explainMode types.ExplainModeV1, trace []*topdown.Event, pretty bool) (explanation types.TraceV1) {
	switch explainMode {
	case types.ExplainNotesV1:
		var err error
		explanation, err = types.NewTraceV1(lineage.Notes(trace), pretty)
		if err != nil {
			break
		}
	case types.ExplainFailsV1:
		var err error
		explanation, err = types.NewTraceV1(lineage.Fails(trace), pretty)
		if err != nil {
			break
		}
	case types.ExplainFullV1:
		var err error
		explanation, err = types.NewTraceV1(trace, pretty)
		if err != nil {
			break
		}
	}
	return explanation
}
