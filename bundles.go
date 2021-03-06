package runtime

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/adler32"
	"sort"
	"strings"

	authz "github.com/aserto-dev/go-grpc-authz/aserto/authorizer/authorizer/v1"
	api "github.com/aserto-dev/go-grpc/aserto/authorizer/policy/v1"
	"github.com/aserto-dev/go-utils/cerr"
	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/server/types"
	"github.com/open-policy-agent/opa/storage"
	"github.com/pkg/errors"
)

type Bundle struct {
	ID   string
	Name string
	Path string
}

func (r *Runtime) GetBundles(ctx context.Context) ([]*api.PolicyItem, error) {
	results := make([]*api.PolicyItem, 0)

	bundles, err := getBundles(ctx, r)
	if err != nil {
		return results, errors.Wrapf(err, "get bundles")
	}

	for _, b := range bundles {
		results = append(results, &api.PolicyItem{
			Id:   b.ID,
			Name: b.Name,
		})
	}
	return results, nil
}

func getBundles(ctx context.Context, r *Runtime) ([]*Bundle, error) {
	const queryStmt = "data.system.bundles[x]"

	queryResults, err := r.Query(ctx, queryStmt, nil, false, false, false, types.ExplainOffV1)
	if err != nil {
		return []*Bundle{}, errors.Wrapf(err, "query bundles")
	}

	results := make([]*Bundle, 0)
	for _, rs := range queryResults.Result {
		v, ok := rs.Bindings["x"].(string)
		if !ok {
			r.Logger.Error().Err(fmt.Errorf("expected binding [x] not found"))
			continue
		}

		path := strings.TrimPrefix(v, "./")

		id := calcID(v)

		name, err := r.GetPolicyRoot(ctx, path)
		if err != nil {
			return []*Bundle{}, errors.Wrapf(err, "get policy name")
		}

		results = append(results, &Bundle{
			ID:   id,
			Name: name,
			Path: path,
		})
	}

	return results, nil
}

func (r *Runtime) GetBundleByID(ctx context.Context, id string) (*Bundle, error) {
	bundles, err := getBundles(ctx, r)
	if err != nil {
		return &Bundle{}, err
	}

	for _, v := range bundles {
		if v.ID == id {
			return v, nil
		}
	}

	return &Bundle{}, cerr.ErrPolicyNotFound.Msg(fmt.Sprintf("bundle for policy id not found [%s]", id))
}

func calcID(v string) string {
	if _, err := uuid.Parse(v); err == nil {
		return v
	}

	return fmt.Sprintf("%d", uint64(adler32.Checksum([]byte(v))))
}

func (r *Runtime) GetPolicies(ctx context.Context, id string) ([]*api.PolicyItem, error) {
	policies := make([]*api.PolicyItem, 0)

	policyList, err := r.GetPolicyList(ctx, id, NoFilter)
	if err != nil {
		return policies, err
	}

	for _, policy := range policyList {
		policies = append(
			policies,
			&api.PolicyItem{
				Name: policy.PackageName,
				Id:   encID(policy.Location),
			},
		)
	}

	// sort policies by their name
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].Name < policies[j].Name
	})

	return policies, nil
}

type Policy struct {
	PackageName string
	Location    string
}

func (p Policy) Name() string {
	s := strings.Split(p.PackageName, ".")
	if len(s) >= 1 {
		return s[0]
	}
	return ""
}

func (p Policy) Package(sep authz.PathSeparator) string {
	switch sep {
	case authz.PathSeparator_PATH_SEPARATOR_DOT:
		return p.PackageName
	case authz.PathSeparator_PATH_SEPARATOR_SLASH:
		return strings.ReplaceAll(p.PackageName, ".", "/")
	default:
		return p.PackageName
	}
}

type PathFilterFn func(packageName string) bool

var NoFilter PathFilterFn = func(packageName string) bool { return true }

func (r *Runtime) PathFilter(sep authz.PathSeparator, path string) PathFilterFn {
	switch sep {
	case authz.PathSeparator_PATH_SEPARATOR_SLASH:
		return func(packageName string) bool {
			if path != "" {
				return strings.HasPrefix(strings.ReplaceAll(packageName, ".", "/"), path)
			}
			return true
		}
	default:
		return func(packageName string) bool {
			if path != "" {
				return strings.HasPrefix(packageName, path)
			}
			return true
		}
	}
}

// GetPolicyList returns the list of policies loaded by the runtime for a given bundle, identified with the policy id.
func (r *Runtime) GetPolicyList(ctx context.Context, id string, fn PathFilterFn) ([]Policy, error) {
	policyList := make([]Policy, 0)

	if fn == nil {
		return policyList, errors.Errorf("path filter is nil")
	}

	bundle, err := r.GetBundleByID(ctx, id)
	if err != nil {
		return []Policy{}, err
	}

	err = storage.Txn(ctx, r.PluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {

		policiesList, errX := r.PluginsManager.Store.ListPolicies(ctx, txn)
		if errX != nil {
			return errors.Wrap(errX, "error listing policies from storage")
		}

		for _, v := range policiesList {
			trimmedPath := strings.TrimPrefix(v, "/")
			trimmedRequestPath := strings.TrimPrefix(bundle.Path, "/")
			// filter out entries which do not belong to policy
			if !strings.HasPrefix(trimmedPath, trimmedRequestPath) {
				continue
			}

			buf, errX := r.PluginsManager.Store.GetPolicy(ctx, txn, v)
			if errX != nil {
				return errors.Wrap(errX, "store.GetPolicy")
			}

			module, errY := ast.ParseModule("", string(buf))
			if errY != nil {
				return errors.Wrap(errY, "ast.ParseModule")
			}

			packageName := strings.TrimPrefix(module.Package.Path.String(), "data.")

			// filter out entries which do prefix the path specified
			if fn != nil && !fn(packageName) {
				continue
			}

			policyList = append(policyList,
				Policy{
					PackageName: packageName,
					Location:    v,
				},
			)
		}
		return nil
	})

	if err != nil {
		return []Policy{}, err
	}

	return policyList, nil
}

// GetPolicyRoot returns the package root name from the policy list (not from the .manifest file).
func (r *Runtime) GetPolicyRoot(ctx context.Context, path string) (string, error) {

	var policyName string

	err := storage.Txn(ctx, r.PluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {

		policiesList, errX := r.PluginsManager.Store.ListPolicies(ctx, txn)
		if errX != nil {
			return errors.Wrap(errX, "error listing policies from storage")
		}

		for _, v := range policiesList {
			// filter out entries which do not belong to policy
			trimmedPath := strings.TrimPrefix(v, "/")
			trimmedRequestPath := strings.TrimPrefix(path, "/")
			if !strings.HasPrefix(trimmedPath, trimmedRequestPath) {
				continue
			}

			buf, errX := r.PluginsManager.Store.GetPolicy(ctx, txn, v)
			if errX != nil {
				return errors.Wrap(errX, "store.GetPolicy")
			}

			module, errY := ast.ParseModule("", string(buf))
			if errY != nil {
				return errors.Wrap(errY, "ast.ParseModule")
			}

			packageName := strings.TrimPrefix(module.Package.Path.String(), "data.")
			s := strings.Split(packageName, ".")
			if len(s) >= 1 {
				policyName = s[0]
				break
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	return policyName, nil
}

// policyExists
func policyExists(ctx context.Context, r *Runtime, id string) bool {
	err := storage.Txn(ctx, r.PluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {
		_, err := r.PluginsManager.Store.GetPolicy(ctx, txn, id)
		return err
	})
	return err == nil
}

func (r *Runtime) GetModule(ctx context.Context, id string) (*api.Module, error) {
	pid := decID(id)

	if !policyExists(ctx, r, pid) {
		return &api.Module{}, cerr.ErrPolicyNotFound.Msg(fmt.Sprintf("policy not found [%s]", pid))
	}

	module, err := getModule(ctx, r, pid)
	if err != nil {
		return &api.Module{}, err
	}

	return module, nil
}

// getModule
func getModule(ctx context.Context, r *Runtime, id string) (*api.Module, error) {
	mod := &api.Module{}

	err := storage.Txn(ctx, r.PluginsManager.Store, storage.TransactionParams{}, func(txn storage.Transaction) error {
		policy, err := r.PluginsManager.Store.GetPolicy(ctx, txn, id)
		if err != nil {
			return errors.Wrap(err, "failed to get policy")
		}

		module, err := ast.ParseModule("", string(policy))
		if err != nil {
			return errors.Wrap(err, "parse module")
		}

		name := strings.TrimPrefix(module.Package.Path.String(), "data.")

		rules := []string{}
		for _, rule := range module.Rules {
			rules = append(rules, rule.Head.Name.String())
		}

		mod.Id = encID(id)
		mod.Name = name
		mod.Content = string(policy)
		mod.Rules = rules

		return nil
	})

	return mod, err
}

// decID decode policy ID (base64 -> string)
func decID(id string) string {
	b, err := base64.URLEncoding.DecodeString(id)
	if err != nil {
		return ""
	}
	return string(b)
}

// encID encode policy ID (string -> base64)
func encID(id string) string {
	return base64.URLEncoding.EncodeToString([]byte(id))
}
