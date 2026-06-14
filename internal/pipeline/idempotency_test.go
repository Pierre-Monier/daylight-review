// ABOUTME: tests for idempotency note formatting and parsing
// ABOUTME: notes are posted as confidential GitLab MR comments to track processed runs
package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatAndParseNote_RoundTrip(t *testing.T) {
	note := FormatNote("abc123", "def456")
	sha, hash, ok := ParseNote(note)

	assert.True(t, ok)
	assert.Equal(t, "abc123", sha)
	assert.Equal(t, "def456", hash)
}

func TestParseNote_InvalidFormat_ReturnsFalse(t *testing.T) {
	_, _, ok := ParseNote("some random comment")

	assert.False(t, ok)
}

func TestParseNote_EmptyBody_ReturnsFalse(t *testing.T) {
	_, _, ok := ParseNote("")

	assert.False(t, ok)
}
