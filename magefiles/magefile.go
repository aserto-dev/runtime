//go:build mage
// +build mage

package main

import (
	"os"

	"github.com/aserto-dev/mage-loot/common"
	"github.com/aserto-dev/mage-loot/deps"
	"github.com/magefile/mage/sh"
)

func init() {
	// Set go version for docker builds
	os.Setenv("GO_VERSION", "1.19")
	// Set private repositories
	os.Setenv("GOPRIVATE", "github.com/aserto-dev")
	// Enable docker buildkit capabilities
	os.Setenv("DOCKER_BUILDKIT", "1")
}

// Lint runs linting for the entire project.
func Lint() error {
	return common.Lint()
}

// Test runs all tests and generates a code coverage report.
func Test() error {
	return common.Test()
}

func Deps() {
	deps.GetAllDeps()
}

// Generate generates all code.
func Generate() error {
	// These extra commands are required because of
	// https://github.com/golang/go/issues/44129

	if err := sh.RunV("go", "get", "-tags", "wireinject", "./..."); err != nil {
		return err
	}
	if err := sh.RunV("go", "mod", "download"); err != nil {
		return err
	}
	if err := common.Generate(); err != nil {
		return err
	}
	if err := sh.RunV("go", "mod", "tidy"); err != nil {
		return err
	}

	return nil
}
