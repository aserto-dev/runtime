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
					"url":                             "https://bundler.eng.aserto.com/790b3ed3-413d-11ec-be40-00bfff9771b6",
					"response_header_timeout_seconds": 5,
					"credentials": map[string]interface{}{
						"bearer": map[string]interface{}{
							"token": "demo",
						},
					},
				},
			},
			Bundles: map[string]*bundle.Source{
				"testbundle": &bundle.Source{
					Service:  "acmecorp",
					Resource: "/f6235144-7e79-11ec-8a01-01bfff9771b6/bundle.tar.gz",
				},
			},
		},
	})

	assert.NoError(err)
	defer cleanup()

	// Act
	err = r.PluginsManager.Start(context.Background())
	assert.NoError(err)
	err = r.WaitForPlugins(context.Background(), time.Second*5)
	assert.NoError(err)
	s := r.Status()

	// Assert
	assert.True(s.Ready)
	assert.Equal(0, len(s.Errors))
	assert.Equal(1, len(s.Bundles))
}

func TestFailedRemoteBundle(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{
		Config: OPAConfig{
			Services: map[string]interface{}{
				"acmecorp": map[string]interface{}{
					"url":                             "https://bundler.eng.aserto.com/6d9fa375-93d3-11eb-a705-002310f5c6bf",
					"response_header_timeout_seconds": 5,
					"credentials": map[string]interface{}{
						"bearer": map[string]interface{}{
							"token": "demo",
						},
					},
				},
			},
			Bundles: map[string]*bundle.Source{
				"testbundle": &bundle.Source{
					Service:  "acmecorp",
					Resource: "/deadbeef/bundle.tar.gz",
				},
			},
		},
	})

	assert.NoError(err)
	defer cleanup()

	// Act
	err = r.PluginsManager.Start(context.Background())
	assert.NoError(err)
	time.Sleep(time.Second * 1)
	s := r.Status()

	// Assert
	assert.False(s.Ready)
	assert.Equal(0, len(s.Errors))
	assert.Equal(1, len(s.Bundles))
	assert.NotEmpty(s.Bundles[0].Errors)
}
