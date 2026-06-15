// ABOUTME: tests for mapping changed file paths to owner candidate pools per section
// ABOUTME: covers prefix matching, multi-section MRs, owner unions, and unowned files
package ownership

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolve_FileMatchesRule(t *testing.T) {
	sections := []Section{
		{Name: "Backend", Rules: []Rule{{Pattern: "/src/", Owners: []string{"alice", "bob"}}}},
	}

	result := Resolve([]string{"/src/main.go"}, sections)

	assert.ElementsMatch(t, []string{"alice", "bob"}, result["Backend"])
}

func TestResolve_MultipleFilesUnionOwners(t *testing.T) {
	sections := []Section{
		{Name: "Backend", Rules: []Rule{
			{Pattern: "/src/", Owners: []string{"alice"}},
			{Pattern: "/tests/", Owners: []string{"bob"}},
		}},
	}

	result := Resolve([]string{"/src/main.go", "/tests/main_test.go"}, sections)

	assert.ElementsMatch(t, []string{"alice", "bob"}, result["Backend"])
}

func TestResolve_MultipleTeams(t *testing.T) {
	sections := []Section{
		{Name: "Frontend", Rules: []Rule{{Pattern: "/ui/", Owners: []string{"carol"}}}},
		{Name: "Backend", Rules: []Rule{{Pattern: "/api/", Owners: []string{"dave"}}}},
	}

	result := Resolve([]string{"/ui/app.js", "/api/server.go"}, sections)

	assert.ElementsMatch(t, []string{"carol"}, result["Frontend"])
	assert.ElementsMatch(t, []string{"dave"}, result["Backend"])
}

func TestResolve_FileWithNoOwnerIgnored(t *testing.T) {
	sections := []Section{
		{Name: "Backend", Rules: []Rule{{Pattern: "/src/", Owners: []string{"alice"}}}},
	}

	result := Resolve([]string{"/docs/readme.md"}, sections)

	assert.Empty(t, result)
}

func TestResolve_FileWithoutLeadingSlash(t *testing.T) {
	sections := []Section{
		{Name: "Backend", Rules: []Rule{{Pattern: "/src/", Owners: []string{"alice"}}}},
	}

	result := Resolve([]string{"src/main.go"}, sections)

	assert.ElementsMatch(t, []string{"alice"}, result["Backend"])
}

func TestResolve_AllFilesUnowned_ReturnsEmptyMap(t *testing.T) {
	result := Resolve([]string{"/docs/readme.md"}, []Section{})

	assert.Empty(t, result)
}
