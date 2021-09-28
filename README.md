# runtime - an abstraction library on top of the Open Policy Agent (OPA)

[![Go Reference](https://pkg.go.dev/badge/github.com/aserto-dev/runtime.svg)](https://pkg.go.dev/github.com/aserto-dev/runtime)
[![Go Report Card](https://goreportcard.com/badge/github.com/aserto-dev/runtime)](https://goreportcard.com/report/github.com/aserto-dev/runtime)

## Introduction

The "runtime" project is a library that sits on top of [OPA](https://github.com/open-policy-agent/opa).

The goal of the project is to allow you to quickly write code that builds, runs or tests OPA policies.

It uses the options pattern to facilitate construction of `Runtime` instances specific to your needs. You can start super simple, using it just to build some rego into a bundle, or you can get more complex, using it to start a runtime with plugins, built-ins and other features.

## Install

```shell
go get -u github.com/aserto-dev/runtime
```

## Usage

```go
// Create a runtime
r, cleanup, err := runtime.NewRuntime(ctx, &logger, &runtime.Config{})
if err != nil {
  return errors.Wrap(err, "failed to create runtime")
}
defer cleanup()

// Use the runtime to build a bundle from the current directory
return r.Build(runtime.BuildParams{
  OutputFile: "my-bundle.tar.gz",
}, ".")
```

You can find a more complete example in the [example](./example/) directory.

## Credits

Based on the awesome [Open Policy Agent](https://github.com/open-policy-agent/opa).
