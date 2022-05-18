package runtime

import (
	"context"

	"github.com/aserto-dev/go-utils/cerr"
	"github.com/google/uuid"
	"github.com/imdario/mergo"
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

type FinalEvaluator func(ctx context.Context, input map[string]interface{}) (*Result, error)

// PartialQuery compiles a query and partially evaluates it, allowing the caller
// to fully evaluate the query at a later time, and for more than one input.
func (r *Runtime) PartialQuery(ctx context.Context, qStr string, input map[string]interface{}, unknowns []string, pretty, includeMetrics, includeInstrumentation bool, explain types.ExplainModeV1) (FinalEvaluator, error) {
	m := metrics.New()

	decisionID := uuid.New().String()

	parsedQuery, err := validateQuery(qStr)
	if err != nil {
		return nil, cerr.ErrBadQuery.Err(err).Str("query", qStr).Msg(
			errors.Wrap(err, "failed to validate query").Error())
	}

	txn, err := r.storage.NewTransaction(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new OPA store transaction")
	}

	defer r.storage.Abort(ctx, txn)

	evaluator, err := r.execPartialQuery(ctx, txn, decisionID, parsedQuery, input, unknowns, m, explain, includeMetrics, includeInstrumentation, pretty)
	if err != nil {
		return nil, cerr.ErrQueryExecutionFailed.
			Str("decision-id", decisionID).
			Str("query", qStr).
			Err(err).
			Msg(errors.Wrap(err, "partial query execution failed").Error())
	}

	return evaluator, nil
}

// Query executes a REGO query against the Aserto OPA Runtime
// explain can be "notes", "full" or "off"
func (r *Runtime) Query(ctx context.Context, qStr string, input map[string]interface{}, pretty, includeMetrics, includeInstrumentation bool, explain types.ExplainModeV1) (*Result, error) {
	m := metrics.New()

	decisionID := uuid.New().String()

	parsedQuery, err := validateQuery(qStr)
	if err != nil {
		return nil, cerr.ErrBadQuery.Err(err).Str("query", qStr).Msg(
			errors.Wrap(err, "failed to validate query").Error())
	}

	txn, err := r.storage.NewTransaction(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new OPA store transaction")
	}

	defer r.storage.Abort(ctx, txn)

	results, err := r.execQuery(ctx, txn, decisionID, parsedQuery, input, m, explain, includeMetrics, includeInstrumentation, pretty)
	if err != nil {
		return nil, cerr.ErrQueryExecutionFailed.
			Str("decision-id", decisionID).
			Str("query", qStr).
			Err(err).
			Msg(errors.Wrap(err, "query execution failed").Error())
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
		rego.Store(r.storage),
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
		rego.Imports(r.imports),
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

func (r *Runtime) execPartialQuery(ctx context.Context, txn storage.Transaction, decisionID string, parsedQuery ast.Body, input map[string]interface{}, unknowns []string, m metrics.Metrics, explainMode types.ExplainModeV1, includeMetrics, includeInstrumentation, pretty bool) (FinalEvaluator, error) {

	var buf *topdown.BufferTracer
	if explainMode != types.ExplainOffV1 {
		buf = topdown.NewBufferTracer()
	}

	opts := r.builtins

	compiler := r.PluginsManager.GetCompiler()

	opts = append(opts,
		rego.Store(r.storage),
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
		rego.Imports(r.imports),
		rego.Unknowns(unknowns),
		rego.Input(input),
	)

	for _, r := range r.PluginsManager.GetWasmResolvers() {
		for _, entrypoint := range r.Entrypoints() {
			opts = append(opts, rego.Resolver(entrypoint, r))
		}
	}

	regoQuery := rego.New(opts...)

	pr, err := regoQuery.PartialResult(ctx)
	if err != nil {
		r.Logger.Warn().
			Err(err).Str("decisionID", decisionID).
			Str("query", parsedQuery.String()).
			Interface("input", input).
			Msg("error partially evaluating query")

		return nil, errors.Wrap(err, "failed to prepare for partial rego query evaluation")
	}

	return func(ctx context.Context, finalInput map[string]interface{}) (*Result, error) {
		err := mergo.Merge(&finalInput, input, mergo.WithOverride)
		if err != nil {
			return nil, errors.Wrap(err, "failed to merge inputs")
		}

		output, err := pr.Rego(
			rego.Input(finalInput),
			rego.Unknowns(unknowns),
		).Eval(ctx)

		if err != nil {
			r.Logger.Warn().
				Err(err).Str("decisionID", decisionID).
				Interface("input", input).
				Msg("error completing query evaluation")

			return nil, errors.Wrap(err, "failed to complete rego query")
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
			Interface("input", input).
			Msg("query evaluation completed")

		return results, err
	}, nil
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
