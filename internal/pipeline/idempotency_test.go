// ABOUTME: tests for idempotency note formatting and parsing
// ABOUTME: notes are posted as confidential GitLab MR comments to track processed runs
package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatAndParseNote_RoundTrip(t *testing.T) {
	assignments := map[string][]string{"Backend Team": {"alice", "bob"}, "Frontend": {"carol"}}

	note := FormatNote("abc123", assignments, nil)
	sha, got, _, ok := ParseNote(note)

	assert.True(t, ok)
	assert.Equal(t, "abc123", sha)
	assert.Equal(t, assignments, got)
}

func TestFormatAndParseNote_EmptyAssignments(t *testing.T) {
	note := FormatNote("abc123", map[string][]string{}, nil)
	sha, got, _, ok := ParseNote(note)

	assert.True(t, ok)
	assert.Equal(t, "abc123", sha)
	assert.Empty(t, got)
}

func TestParseNote_InvalidFormat_ReturnsFalse(t *testing.T) {
	_, _, _, ok := ParseNote("some random comment")

	assert.False(t, ok)
}

func TestParseNote_EmptyBody_ReturnsFalse(t *testing.T) {
	_, _, _, ok := ParseNote("")

	assert.False(t, ok)
}

func TestFormatParseNote_RoundTripsOOO(t *testing.T) {
	body := FormatNote("sha1", map[string][]string{"Backend": {"bob"}}, []string{"alice"})
	sha, assignments, ooo, ok := ParseNote(body)
	assert.True(t, ok)
	assert.Equal(t, "sha1", sha)
	assert.Equal(t, map[string][]string{"Backend": {"bob"}}, assignments)
	assert.Equal(t, []string{"alice"}, ooo)
}

func TestParseNote_LegacyNoteWithoutOOO(t *testing.T) {
	body := "daylight:processed " + `{"sha":"sha1","assignments":{"Backend":["bob"]}}`
	sha, assignments, ooo, ok := ParseNote(body)
	assert.True(t, ok)
	assert.Equal(t, "sha1", sha)
	assert.Equal(t, map[string][]string{"Backend": {"bob"}}, assignments)
	assert.Empty(t, ooo)
}
