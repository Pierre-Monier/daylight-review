// ABOUTME: tests for CODEOWNERS file parsing into sections with rules and owners
// ABOUTME: covers both owner syntaxes, multiple sections, comments, and blank lines
package ownership

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse_PatternA_OwnersOnRuleLine(t *testing.T) {
	content := "[Backend][1]\n/src/ @alice @bob\n/tests/ @carol\n"

	sections := Parse(content)

	assert.Len(t, sections, 1)
	assert.Equal(t, "Backend", sections[0].Name)
	assert.Empty(t, sections[0].DefaultOwners)
	assert.Len(t, sections[0].Rules, 2)
	assert.Equal(t, "/src/", sections[0].Rules[0].Pattern)
	assert.ElementsMatch(t, []string{"alice", "bob"}, sections[0].Rules[0].Owners)
	assert.Equal(t, "/tests/", sections[0].Rules[1].Pattern)
	assert.ElementsMatch(t, []string{"carol"}, sections[0].Rules[1].Owners)
}

func TestParse_PatternB_DefaultOwnersOnSectionHeader(t *testing.T) {
	content := "[Backend][1] @alice @bob\n/src/\n/tests/\n"

	sections := Parse(content)

	assert.Len(t, sections, 1)
	assert.ElementsMatch(t, []string{"alice", "bob"}, sections[0].DefaultOwners)
	assert.Len(t, sections[0].Rules, 2)
	assert.ElementsMatch(t, []string{"alice", "bob"}, sections[0].Rules[0].Owners)
	assert.ElementsMatch(t, []string{"alice", "bob"}, sections[0].Rules[1].Owners)
}

func TestParse_MultipleSections(t *testing.T) {
	content := "[Frontend][1]\n/ui/ @dave\n[Backend][2]\n/api/ @eve\n"

	sections := Parse(content)

	assert.Len(t, sections, 2)
	assert.Equal(t, "Frontend", sections[0].Name)
	assert.Equal(t, "Backend", sections[1].Name)
}

func TestParse_CommentsAndBlankLinesIgnored(t *testing.T) {
	content := "# This is a comment\n[Backend][1]\n# Another comment\n\n/src/ @alice\n"

	sections := Parse(content)

	assert.Len(t, sections, 1)
	assert.Len(t, sections[0].Rules, 1)
}

func TestParse_RuleWithNoOwnersAndNoSectionDefaults(t *testing.T) {
	content := "[Backend][1]\n/src/\n"

	sections := Parse(content)

	assert.Empty(t, sections[0].Rules[0].Owners)
}
