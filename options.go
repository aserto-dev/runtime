package runtime

import (
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/plugins"
	"github.com/open-policy-agent/opa/rego"
)

type RuntimeOption func(*Runtime)

func WithPlugin(name string, factory plugins.Factory) RuntimeOption {
	return func(r *Runtime) {
		r.plugins[name] = factory
	}
}

func WithBuiltin1(decl *rego.Function, impl rego.Builtin1) RuntimeOption {
	return func(r *Runtime) {
		r.builtins1[decl] = impl
		r.builtins = append(r.builtins, rego.Function1(decl, impl))
		r.compilerBuiltins[decl.Name] = &ast.Builtin{
			Name: decl.Name,
			Decl: decl.Decl,
		}
	}
}

func WithBuiltin2(decl *rego.Function, impl rego.Builtin2) RuntimeOption {
	return func(r *Runtime) {
		r.builtins2[decl] = impl
		r.builtins = append(r.builtins, rego.Function2(decl, impl))
		r.compilerBuiltins[decl.Name] = &ast.Builtin{
			Name: decl.Name,
			Decl: decl.Decl,
		}
	}
}

func WithBuiltin3(decl *rego.Function, impl rego.Builtin3) RuntimeOption {
	return func(r *Runtime) {
		r.builtins3[decl] = impl
		r.builtins = append(r.builtins, rego.Function3(decl, impl))
		r.compilerBuiltins[decl.Name] = &ast.Builtin{
			Name: decl.Name,
			Decl: decl.Decl,
		}
	}
}

func WithBuiltin4(decl *rego.Function, impl rego.Builtin4) RuntimeOption {
	return func(r *Runtime) {
		r.builtins4[decl] = impl
		r.builtins = append(r.builtins, rego.Function4(decl, impl))
		r.compilerBuiltins[decl.Name] = &ast.Builtin{
			Name: decl.Name,
			Decl: decl.Decl,
		}
	}
}

func WithBuiltinDyn(decl *rego.Function, impl rego.BuiltinDyn) RuntimeOption {
	return func(r *Runtime) {
		r.builtinsDyn[decl] = impl
		r.builtins = append(r.builtins, rego.FunctionDyn(decl, impl))
		r.compilerBuiltins[decl.Name] = &ast.Builtin{
			Name: decl.Name,
			Decl: decl.Decl,
		}
	}
}
