package runtime_test

import (
	"context"
	"testing"
	"time"

	runtime "github.com/aserto-dev/runtime"
	"github.com/aserto-dev/runtime/testutil"
	"github.com/open-policy-agent/opa/v1/plugins/bundle"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestEmptyRuntime(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := runtime.NewRuntime(context.Background(), &zerolog.Logger{}, &runtime.Config{})
	assert.NoError(err)

	defer cleanup()

	// Act
	s := r.Status()

	// Assert
	assert.True(s.Ready)
}

func TestLocalBundle(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := runtime.NewRuntime(context.Background(), &zerolog.Logger{}, &runtime.Config{
		LocalBundles: runtime.LocalBundlesConfig{
			Paths: []string{testutil.AssetSimpleBundle()},
		},
	})
	assert.NoError(err)

	defer cleanup()

	// Act
	s := r.Status()

	// Assert
	assert.True(s.Ready)
	assert.Empty(s.Errors)
	assert.Len(s.Bundles, 1)
}

func TestFailingLocalBundle(t *testing.T) {
	// Arrange
	assert := require.New(t)

	// Act
	_, _, err := runtime.NewRuntime(context.Background(), &zerolog.Logger{}, &runtime.Config{
		LocalBundles: runtime.LocalBundlesConfig{
			Paths: []string{testutil.AssetBuiltinsBundle()},
		},
	})

	// Assert
	assert.Error(err)
}

func TestRemoteBundle(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := runtime.NewRuntime(context.Background(), &zerolog.Logger{}, &runtime.Config{
		Config: runtime.OPAConfig{
			Services: map[string]interface{}{
				"acmecorp": map[string]interface{}{
					"url":                             "https://ghcr.io",
					"response_header_timeout_seconds": 5,
					"type":                            "oci",
				},
			},
			Bundles: map[string]*bundle.Source{
				"testbundle": {
					Service:  "acmecorp",
					Resource: "ghcr.io/aserto-policies/policy-peoplefinder-rbac:2",
				},
			},
		},
	})

	assert.NoError(err)

	defer cleanup()

	// Act
	err = r.Start(context.Background())
	assert.NoError(err)

	err = r.WaitForPlugins(context.Background(), time.Second*5)
	assert.NoError(err)

	s := r.Status()

	// Assert
	assert.True(s.Ready)
	assert.Empty(s.Errors)
	assert.Len(s.Bundles, 1)
}
