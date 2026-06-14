// ABOUTME: tests for the random reviewer selection strategy
// ABOUTME: verifies author exclusion, empty pool, and pool-containing-only-author behavior
package selection

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomStrategy_SelectsFromPool(t *testing.T) {
	s := RandomStrategy{}
	candidates := []string{"alice", "bob", "carol"}

	result, err := s.Select(candidates, "nobody")

	require.NoError(t, err)
	assert.Contains(t, candidates, result)
}

func TestRandomStrategy_ExcludesAuthor(t *testing.T) {
	s := RandomStrategy{}
	candidates := []string{"alice", "author"}

	for i := 0; i < 20; i++ {
		result, err := s.Select(candidates, "author")
		require.NoError(t, err)
		assert.Equal(t, "alice", result)
	}
}

func TestRandomStrategy_EmptyPool_ReturnsError(t *testing.T) {
	s := RandomStrategy{}

	_, err := s.Select([]string{}, "author")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoEligibleReviewer))
}

func TestRandomStrategy_OnlyAuthorInPool_ReturnsError(t *testing.T) {
	s := RandomStrategy{}

	_, err := s.Select([]string{"author"}, "author")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoEligibleReviewer))
}
