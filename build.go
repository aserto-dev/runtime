package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/bundle"
	"github.com/open-policy-agent/opa/v1/compile"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/topdown"
	"github.com/open-policy-agent/opa/v1/types"
	"github.com/pkg/errors"
)

// BuildTargetType represents the type of build target.
type BuildTargetType int

const (
	Rego BuildTargetType = iota
	Wasm
)

type fakeBuiltin struct {
	Name string         `json:"name"`
	Decl types.Function `json:"decl"`
}

type (
	fakeBuiltin1   fakeBuiltin
	fakeBuiltin2   fakeBuiltin
	fakeBuiltin3   fakeBuiltin
	fakeBuiltin4   fakeBuiltin
	fakeBuiltinDyn fakeBuiltin
)

type fakeBuiltinDefs struct {
	Builtin1   []fakeBuiltin1   `json:"builtin1,omitempty"`
	Builtin2   []fakeBuiltin2   `json:"builtin2,omitempty"`
	Builtin3   []fakeBuiltin3   `json:"builtin3,omitempty"`
	Builtin4   []fakeBuiltin4   `json:"builtin4,omitempty"`
	BuiltinDyn []fakeBuiltinDyn `json:"builtinDyn,omitempty"`
}

func (t BuildTargetType) String() string {
	return buildTargetTypeToString[t]
}

var buildTargetTypeToString = map[BuildTargetType]string{
	Rego: "rego",
	Wasm: "wasm",
}

// BuildParams contains all parameters used for doing a build.
type BuildParams struct {
	CapabilitiesJSONFile string
	Target               BuildTargetType
	OptimizationLevel    int
	Entrypoints          []string
	OutputFile           string
	Revision             string
	Ignore               []string
	Debug                bool
	Algorithm            string
	Key                  string
	Scope                string
	PubKey               string
	PubKeyID             string
	ClaimsFile           string
	ExcludeVerifyFiles   []string
	RegoV1               bool
}

// Build builds a bundle using the Aserto OPA Runtime.
func (r *Runtime) Build(params *BuildParams, paths []string) error {
	buf := bytes.NewBuffer(nil)

	err := r.generateAllFakeBuiltins(paths)
	if err != nil {
		return err
	}

	// generate the bundle verification and signing config.
	var bvc *bundle.VerificationConfig
	if params.PubKey != "" {
		bvc, err = buildVerificationConfig(params.PubKey, params.PubKeyID, params.Algorithm, params.Scope, params.ExcludeVerifyFiles)
		if err != nil {
			return err
		}
	}

	bsc := buildSigningConfig(params.Key, params.Algorithm, params.ClaimsFile)

	var capabilities *ast.Capabilities
	// if capabilities are not provided then ast.CapabilitiesForThisVersion must be used.
	if params.CapabilitiesJSONFile == "" {
		capabilities = ast.CapabilitiesForThisVersion()
	} else {
		capabilitiesJSON, err := os.ReadFile(params.CapabilitiesJSONFile)
		if err != nil {
			return errors.Wrapf(err, "couldn't read capabilities JSON file [%s]", params.CapabilitiesJSONFile)
		}
		capabilities, err = ast.LoadCapabilitiesJSON(bytes.NewBuffer(capabilitiesJSON))
		if err != nil {
			return errors.Wrapf(err, "failed to load capabilities file [%s]", params.CapabilitiesJSONFile)
		}
	}

	compiler := compile.New().
		WithCapabilities(capabilities).
		WithTarget(params.Target.String()).
		WithAsBundle(true).
		WithOptimizationLevel(params.OptimizationLevel).
		WithOutput(buf).
		WithEntrypoints(params.Entrypoints...).
		WithPaths(paths...).
		WithFilter(buildCommandLoaderFilter(true, params.Ignore)).
		WithRevision(params.Revision).
		WithBundleVerificationConfig(bvc).
		WithBundleSigningConfig(bsc)

	if params.RegoV1 {
		compiler = compiler.WithRegoVersion(ast.RegoV1)
	} else {
		compiler = compiler.WithRegoVersion(ast.RegoV0)
	}

	if params.ClaimsFile == "" {
		compiler = compiler.WithBundleVerificationKeyID(params.PubKeyID)
	}

	err = compiler.Build(context.Background())
	if err != nil {
		return err
	}

	out, err := os.Create(params.OutputFile)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, buf)
	if err != nil {
		return err
	}

	return out.Close()
}

func buildCommandLoaderFilter(bundleMode bool, ignore []string) func(string, os.FileInfo, int) bool {
	return func(abspath string, info os.FileInfo, depth int) bool {
		if !bundleMode {
			if !info.IsDir() && strings.HasSuffix(abspath, ".tar.gz") {
				return true
			}
		}
		return loaderFilter{Ignore: ignore}.Apply(abspath, info, depth)
	}
}

func buildVerificationConfig(pubKey, pubKeyID, alg, scope string, excludeFiles []string) (*bundle.VerificationConfig, error) {
	if pubKey == "" {
		return nil, errors.New("pubKey is empty")
	}

	keyConfig := &bundle.KeyConfig{
		Key:       pubKey,
		Algorithm: alg,
		Scope:     scope,
	}

	return bundle.NewVerificationConfig(map[string]*bundle.KeyConfig{pubKeyID: keyConfig}, pubKeyID, scope, excludeFiles), nil
}

func buildSigningConfig(key, alg, claimsFile string) *bundle.SigningConfig {
	if key == "" {
		return nil
	}

	return bundle.NewSigningConfig(key, alg, claimsFile)
}

func (r *Runtime) registerFakeBuiltins(defs *fakeBuiltinDefs) {
	for _, b := range defs.Builtin1 {
		builtin := b
		if topdown.GetBuiltin(b.Name) != nil {
			r.Logger.Info().Str("builtin", b.Name).Msg("Builtin already declared, skipping fake declaration.")
		}

		rego.RegisterBuiltin1(&rego.Function{
			Name:    builtin.Name,
			Memoize: false,
			Decl:    &builtin.Decl,
		}, func(rego.BuiltinContext, *ast.Term) (*ast.Term, error) {
			return nil, nil
		})
	}

	for _, b := range defs.Builtin2 {
		builtin := b
		if topdown.GetBuiltin(b.Name) != nil {
			r.Logger.Info().Str("builtin", b.Name).Msg("Builtin already declared, skipping fake declaration.")
		}

		rego.RegisterBuiltin2(&rego.Function{
			Name:    builtin.Name,
			Memoize: false,
			Decl:    &builtin.Decl,
		}, func(bctx rego.BuiltinContext, op1, op2 *ast.Term) (*ast.Term, error) {
			return nil, nil
		})
	}

	for _, b := range defs.Builtin3 {
		builtin := b
		if topdown.GetBuiltin(b.Name) != nil {
			r.Logger.Info().Str("builtin", b.Name).Msg("Builtin already declared, skipping fake declaration.")
		}

		rego.RegisterBuiltin3(&rego.Function{
			Name:    builtin.Name,
			Memoize: false,
			Decl:    &builtin.Decl,
		}, func(bctx rego.BuiltinContext, op1, op2, op3 *ast.Term) (*ast.Term, error) {
			return nil, nil
		})
	}

	for _, b := range defs.Builtin4 {
		builtin := b
		if topdown.GetBuiltin(b.Name) != nil {
			r.Logger.Info().Str("builtin", b.Name).Msg("Builtin already declared, skipping fake declaration.")
		}

		rego.RegisterBuiltin4(&rego.Function{
			Name:    builtin.Name,
			Memoize: false,
			Decl:    &builtin.Decl,
		}, func(bctx rego.BuiltinContext, op1, op2, op3, op4 *ast.Term) (*ast.Term, error) {
			return nil, nil
		})
	}

	for _, b := range defs.BuiltinDyn {
		builtin := b
		if topdown.GetBuiltin(b.Name) != nil {
			r.Logger.Info().Str("builtin", b.Name).Msg("Builtin already declared, skipping fake declaration.")
		}

		rego.RegisterBuiltinDyn(&rego.Function{
			Name:    builtin.Name,
			Memoize: false,
			Decl:    &builtin.Decl,
		}, func(bctx rego.BuiltinContext, terms []*ast.Term) (*ast.Term, error) {
			return nil, nil
		})
	}
}

func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, errors.Wrapf(err, "failed to stat file '%s'", path)
	}
}

func (r *Runtime) generateAllFakeBuiltins(paths []string) error {
	for _, path := range paths {
		manifestPath := filepath.Join(path, ".manifest")
		manifestExists, err := fileExists(manifestPath)
		if err != nil {
			return errors.Wrapf(err, "failed to determine if file [%s] exists", manifestPath)
		}

		if !manifestExists {
			continue
		}

		manifestBytes, err := os.ReadFile(manifestPath)
		if err != nil {
			return errors.Wrapf(err, "failed to read manifest [%s]", manifestPath)
		}

		manifest := struct {
			Metadata struct {
				RequiredBuiltins *fakeBuiltinDefs `json:"required_builtins"`
			} `json:"metadata,omitempty"`
		}{}
		err = json.Unmarshal(manifestBytes, &manifest)
		if err != nil {
			return errors.Wrapf(err, "failed to unmarshal json from manifest [%s]", manifestPath)
		}

		if manifest.Metadata.RequiredBuiltins != nil {
			r.registerFakeBuiltins(manifest.Metadata.RequiredBuiltins)
		}
	}

	return nil
}
