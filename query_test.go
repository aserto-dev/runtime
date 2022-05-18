package runtime

import (
	"context"
	"testing"

	"github.com/aserto-dev/runtime/testutil"
	"github.com/open-policy-agent/opa/server/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestSimpleQuery(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{
		LocalBundles: LocalBundlesConfig{
			Paths: []string{testutil.AssetMycarsBundle()},
		},
	})
	assert.NoError(err)
	defer cleanup()

	// Act
	result, err := r.Query(
		context.Background(),
		"x=data",
		map[string]interface{}{},
		false,
		false,
		false,
		types.ExplainOffV1,
	)

	// Assert
	assert.NoError(err)
	assert.Greater(len(result.Result), 0)
}

func TestQueryNotAllowed(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{
		LocalBundles: LocalBundlesConfig{
			Paths: []string{testutil.AssetPartialsBundle()},
		},
	})
	assert.NoError(err)
	defer cleanup()

	// Act
	result, err := r.Query(
		context.Background(),
		"x=data.partials.allowed",
		map[string]interface{}{
			"role":   "viewer",
			"action": "delete",
		}, false,
		false,
		false,
		types.ExplainOffV1,
	)

	// Assert
	assert.NoError(err)
	assert.Equal(false, result.Result[0].Bindings["x"])
}

func TestQueryAllowed(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{
		LocalBundles: LocalBundlesConfig{
			Paths: []string{testutil.AssetPartialsBundle()},
		},
	})
	assert.NoError(err)
	defer cleanup()

	// Act
	result, err := r.Query(
		context.Background(),
		"x=data.partials.allowed",
		map[string]interface{}{
			"role":   "admin",
			"action": "delete",
		},
		false,
		false,
		false,
		types.ExplainOffV1,
	)

	// Assert
	assert.NoError(err)
	assert.Equal(true, result.Result[0].Bindings["x"])
}

func TestPartialQuery(t *testing.T) {
	// Arrange
	assert := require.New(t)
	r, cleanup, err := NewRuntime(context.Background(), &zerolog.Logger{}, &Config{
		LocalBundles: LocalBundlesConfig{
			Paths: []string{testutil.AssetPartialsBundle()},
		},
	})
	assert.NoError(err)
	defer cleanup()

	// Act
	evaluator, err := r.PartialQuery(
		context.Background(),
		"data.partials.allowed",
		map[string]interface{}{
			"action": "delete",
		},
		[]string{"input.role"},
		false,
		false,
		false,
		types.ExplainOffV1,
	)
	assert.NoError(err)

	for role, expectedAllowed := range map[string]bool{
		"admin":  true,
		"user":   false,
		"viewer": false,
	} {
		result, err := evaluator(context.Background(), map[string]interface{}{
			"role":   role,
			"action": "view",
		})

		// Assert
		assert.NoError(err)
		assert.Equal(expectedAllowed, result.Result[0].Expressions[0].Value)
	}
}
