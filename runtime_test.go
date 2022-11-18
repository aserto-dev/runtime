package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/aserto-dev/runtime/testutil"
	"github.com/open-policy-agent/opa/plugins/bundle"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestEmptyRuntime(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{})
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
	r, cleanup, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{
		LocalBundles: LocalBundlesConfig{
			Paths: []string{testutil.AssetSimpleBundle()},
		},
	})
	assert.NoError(err)
	defer cleanup()

	// Act
	s := r.Status()

	// Assert
	assert.True(s.Ready)
	assert.Equal(0, len(s.Errors))
	assert.Equal(1, len(s.Bundles))
}

func TestFailingLocalBundle(t *testing.T) {
	// Arrange
	assert := require.New(t)

	// Act
	_, _, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{
		LocalBundles: LocalBundlesConfig{
			Paths: []string{testutil.AssetBuiltinsBundle()},
		},
	})

	// Assert
	assert.Error(err)
}

func TestRemoteBundle(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{
		Config: OPAConfig{
			Services: map[string]interface{}{
				"acmecorp": map[string]interface{}{
					"url":                             "https://opcr.io",
					"response_header_timeout_seconds": 5,
					"type":                            "oci",
					"credentials": map[string]interface{}{
						"bearer": map[string]interface{}{
							"token": "iDog",
						},
					},
				},
			},
			Bundles: map[string]*bundle.Source{
				"testbundle": &bundle.Source{
					Service:  "acmecorp",
					Resource: "opcr.io/public-test-images/peoplefinder:1.0.0",
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
	assert.Equal(0, len(s.Errors))
	assert.Equal(1, len(s.Bundles))
}
