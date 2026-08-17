// ABOUTME: tests for parsing out-of-office command notes and building the OOO username set
// ABOUTME: pure unit tests, no network
package pipeline

import (
	"testing"

	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/stretchr/testify/assert"
)

func TestParseOOOCommand(t *testing.T) {
	cases := []struct {
		body     string
		wantUser string
		wantOK   bool
	}{
		{"daylight:ooo @alice", "alice", true},
		{"daylight:ooo alice", "alice", true},
		{"  daylight:ooo   @bob  ", "bob", true},
		{"daylight:processed {}", "", false},
		{"just a comment", "", false},
		{"daylight:ooo", "", false},
	}
	for _, c := range cases {
		user, ok := ParseOOOCommand(c.body)
		assert.Equal(t, c.wantOK, ok, "body %q", c.body)
		assert.Equal(t, c.wantUser, user, "body %q", c.body)
	}
}

func TestRequestedOOO_DedupesAndSorts(t *testing.T) {
	notes := []gitlab.Note{
		{Body: "daylight:ooo @bob"},
		{Body: "daylight:ooo @alice"},
		{Body: "daylight:ooo alice"},
		{Body: "unrelated"},
	}
	assert.Equal(t, []string{"alice", "bob"}, requestedOOO(notes))
}

func TestUnionOOO_MergesSortedDeduped(t *testing.T) {
	assert.Equal(t, []string{"alice", "bob", "carol"},
		unionOOO([]string{"bob", "alice"}, []string{"carol", "alice"}))
}
