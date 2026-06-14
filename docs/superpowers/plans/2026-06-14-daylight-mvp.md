# Daylight Review MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI binary (`daylight assign`) that runs as a GitLab CI job and
automatically assigns reviewers to merge requests based on CODEOWNERS ownership.

**Architecture:** Stateless Go binary reading from CI env vars. Runs as a GitLab CI pipeline
job triggered on `merge_request_event`. Idempotency stored as confidential MR notes in GitLab
itself — no database required. Pipeline: draft check → idempotency check → fetch files →
resolve CODEOWNERS → select reviewer per team → assign → post note.

**Tech Stack:** Go 1.23, `net/http` (stdlib), `encoding/json` (stdlib),
`github.com/stretchr/testify` for tests.

---

## File Structure

```
go.mod
go.sum
Dockerfile
.gitlab-ci.yml
cmd/daylight/main.go                     — entry point: load config, run pipeline
internal/config/config.go                — read + validate CI env vars
internal/config/config_test.go
internal/gitlab/client.go                — GitLabClient interface + HTTP implementation
internal/gitlab/client_test.go           — integration tests (skipped by default)
internal/ownership/parser.go             — CODEOWNERS parser → []Section
internal/ownership/parser_test.go
internal/ownership/resolver.go           — map changed files → {team: []username}
internal/ownership/resolver_test.go
internal/selection/strategy.go           — SelectionStrategy interface + ErrNoEligibleReviewer
internal/selection/random.go             — RandomStrategy: picks a random non-author candidate
internal/selection/random_test.go
internal/pipeline/idempotency.go         — FormatNote / ParseNote helpers
internal/pipeline/idempotency_test.go
internal/pipeline/pipeline.go            — Run(): orchestrates the 9-step assignment pipeline
internal/pipeline/pipeline_test.go
```

---

### Task 1: Project Scaffold

**Files:**
- Create: `go.mod`, `go.sum`

- [ ] **Step 1: Create directory structure and initialize Go module**

```bash
mkdir -p cmd/daylight \
  internal/config \
  internal/gitlab \
  internal/ownership \
  internal/selection \
  internal/pipeline
go mod init github.com/daylight-review/daylight
go get github.com/stretchr/testify@latest
```

- [ ] **Step 2: Verify**

```bash
cat go.mod
```

Expected: first line is `module github.com/daylight-review/daylight`, go version `1.23`.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: initialize Go module"
```

---

### Task 2: Config

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
// ABOUTME: tests for CI environment variable parsing and validation
// ABOUTME: verifies required vars, draft detection, and GitLab URL fallback logic
package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withEnv(vars map[string]string) func() {
	for k, v := range vars {
		os.Setenv(k, v)
	}
	return func() {
		for k := range vars {
			os.Unsetenv(k)
		}
	}
}

func requiredVars() map[string]string {
	return map[string]string{
		"CI_PROJECT_ID":         "123",
		"CI_MERGE_REQUEST_IID":  "42",
		"CI_COMMIT_SHA":         "abc123",
		"DAYLIGHT_GITLAB_TOKEN": "secret",
	}
}

func TestLoad_AllRequired(t *testing.T) {
	vars := requiredVars()
	vars["CI_SERVER_URL"] = "https://gitlab.example.com"
	defer withEnv(vars)()

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "123", cfg.ProjectID)
	assert.Equal(t, "42", cfg.MRIID)
	assert.Equal(t, "abc123", cfg.CommitSHA)
	assert.Equal(t, "https://gitlab.example.com", cfg.GitLabURL)
	assert.Equal(t, "secret", cfg.Token)
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Unsetenv("CI_PROJECT_ID")
	os.Unsetenv("CI_MERGE_REQUEST_IID")
	os.Unsetenv("CI_COMMIT_SHA")
	os.Unsetenv("DAYLIGHT_GITLAB_TOKEN")

	_, err := Load()

	require.Error(t, err)
}

func TestLoad_IsDraftTrue(t *testing.T) {
	vars := requiredVars()
	vars["CI_MERGE_REQUEST_DRAFT"] = "true"
	defer withEnv(vars)()

	cfg, err := Load()

	require.NoError(t, err)
	assert.True(t, cfg.IsDraft)
}

func TestLoad_URLFallbackToGitLabCom(t *testing.T) {
	defer withEnv(requiredVars())()
	os.Unsetenv("CI_SERVER_URL")
	os.Unsetenv("DAYLIGHT_GITLAB_URL")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com", cfg.GitLabURL)
}

func TestLoad_DAYLIGHTURLOverridesCIServerURL(t *testing.T) {
	vars := requiredVars()
	vars["CI_SERVER_URL"] = "https://gitlab.example.com"
	vars["DAYLIGHT_GITLAB_URL"] = "https://custom.gitlab.com"
	defer withEnv(vars)()

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "https://custom.gitlab.com", cfg.GitLabURL)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/...
```

Expected: compilation error — `config.Load` undefined.

- [ ] **Step 3: Implement config**

```go
// internal/config/config.go
// ABOUTME: reads and validates CI environment variables required by the pipeline
// ABOUTME: returns a Config struct or an error if any required variable is missing
package config

import (
	"fmt"
	"os"
)

type Config struct {
	ProjectID string
	MRIID     string
	CommitSHA string
	IsDraft   bool
	GitLabURL string
	Token     string
}

func Load() (Config, error) {
	required := []string{
		"CI_PROJECT_ID",
		"CI_MERGE_REQUEST_IID",
		"CI_COMMIT_SHA",
		"DAYLIGHT_GITLAB_TOKEN",
	}
	for _, k := range required {
		if os.Getenv(k) == "" {
			return Config{}, fmt.Errorf("missing required env var: %s", k)
		}
	}

	gitlabURL := os.Getenv("DAYLIGHT_GITLAB_URL")
	if gitlabURL == "" {
		gitlabURL = os.Getenv("CI_SERVER_URL")
	}
	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}

	return Config{
		ProjectID: os.Getenv("CI_PROJECT_ID"),
		MRIID:     os.Getenv("CI_MERGE_REQUEST_IID"),
		CommitSHA: os.Getenv("CI_COMMIT_SHA"),
		IsDraft:   os.Getenv("CI_MERGE_REQUEST_DRAFT") == "true",
		GitLabURL: gitlabURL,
		Token:     os.Getenv("DAYLIGHT_GITLAB_TOKEN"),
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/...
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config loading from CI env vars"
```

---

### Task 3: CODEOWNERS Parser

**Files:**
- Create: `internal/ownership/parser.go`
- Create: `internal/ownership/parser_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ownership/parser_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ownership/... -run TestParse
```

Expected: compilation error — `ownership.Parse` undefined.

- [ ] **Step 3: Implement the parser**

```go
// internal/ownership/parser.go
// ABOUTME: parses GitLab CODEOWNERS file format into sections with rules and owner lists
// ABOUTME: strips @ prefix from usernames; rule owners fall back to section defaults if absent
package ownership

import "strings"

type Section struct {
	Name          string
	DefaultOwners []string
	Rules         []Rule
}

type Rule struct {
	Pattern string
	Owners  []string
}

func Parse(content string) []Section {
	var sections []Section
	var current *Section

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			s := parseSection(line)
			sections = append(sections, s)
			current = &sections[len(sections)-1]
			continue
		}
		if current != nil && strings.HasPrefix(line, "/") {
			current.Rules = append(current.Rules, parseRule(line, current.DefaultOwners))
		}
	}
	return sections
}

func parseSection(line string) Section {
	end := strings.Index(line, "]")
	name := line[1:end]
	rest := line[end+1:]
	if strings.HasPrefix(rest, "[") {
		rest = rest[strings.Index(rest, "]")+1:]
	}
	return Section{
		Name:          name,
		DefaultOwners: parseOwners(strings.TrimSpace(rest)),
	}
}

func parseRule(line string, sectionDefaults []string) Rule {
	parts := strings.Fields(line)
	pattern := parts[0]
	if len(parts) > 1 {
		return Rule{Pattern: pattern, Owners: parseOwners(strings.Join(parts[1:], " "))}
	}
	owners := make([]string, len(sectionDefaults))
	copy(owners, sectionDefaults)
	return Rule{Pattern: pattern, Owners: owners}
}

func parseOwners(s string) []string {
	if s == "" {
		return nil
	}
	fields := strings.Fields(s)
	owners := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.HasPrefix(f, "@") {
			owners = append(owners, f[1:])
		}
	}
	return owners
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ownership/... -run TestParse
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ownership/parser.go internal/ownership/parser_test.go
git commit -m "feat: add CODEOWNERS parser"
```

---

### Task 4: Ownership Resolver

**Files:**
- Create: `internal/ownership/resolver.go`
- Create: `internal/ownership/resolver_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ownership/resolver_test.go
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

func TestResolve_AllFilesUnowned_ReturnsEmptyMap(t *testing.T) {
	result := Resolve([]string{"/docs/readme.md"}, []Section{})

	assert.Empty(t, result)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ownership/... -run TestResolve
```

Expected: compilation error — `ownership.Resolve` undefined.

- [ ] **Step 3: Implement the resolver**

```go
// internal/ownership/resolver.go
// ABOUTME: maps a list of changed file paths to candidate reviewer pools per owning section
// ABOUTME: uses prefix matching; unions owners from all matched rules within each section
package ownership

import "strings"

// Resolve returns a map of section name to candidate usernames for the given changed files.
// Files with no matching rule are ignored. Returns an empty map if nothing has an owner.
func Resolve(changedFiles []string, sections []Section) map[string][]string {
	seen := make(map[string]map[string]bool)

	for _, file := range changedFiles {
		for _, section := range sections {
			for _, rule := range section.Rules {
				if strings.HasPrefix(file, rule.Pattern) {
					if seen[section.Name] == nil {
						seen[section.Name] = make(map[string]bool)
					}
					for _, owner := range rule.Owners {
						seen[section.Name][owner] = true
					}
				}
			}
		}
	}

	result := make(map[string][]string, len(seen))
	for name, ownerSet := range seen {
		owners := make([]string, 0, len(ownerSet))
		for owner := range ownerSet {
			owners = append(owners, owner)
		}
		result[name] = owners
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ownership/...
```

Expected: all tests PASS (parser + resolver).

- [ ] **Step 5: Commit**

```bash
git add internal/ownership/resolver.go internal/ownership/resolver_test.go
git commit -m "feat: add ownership resolver"
```

---

### Task 5: SelectionStrategy + RandomStrategy

**Files:**
- Create: `internal/selection/strategy.go`
- Create: `internal/selection/random.go`
- Create: `internal/selection/random_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/selection/random_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/selection/...
```

Expected: compilation error — `selection.RandomStrategy` undefined.

- [ ] **Step 3: Implement the strategy interface and RandomStrategy**

```go
// internal/selection/strategy.go
// ABOUTME: defines the reviewer selection interface and the sentinel error for empty pools
package selection

import "errors"

// ErrNoEligibleReviewer is returned when the candidate pool is empty or only contains the author.
var ErrNoEligibleReviewer = errors.New("no eligible reviewer")

type SelectionStrategy interface {
	Select(candidates []string, exclude string) (string, error)
}
```

```go
// internal/selection/random.go
// ABOUTME: implements SelectionStrategy by picking a uniformly random candidate excluding the author
package selection

import "math/rand"

type RandomStrategy struct{}

func (RandomStrategy) Select(candidates []string, exclude string) (string, error) {
	eligible := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c != exclude {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return "", ErrNoEligibleReviewer
	}
	return eligible[rand.Intn(len(eligible))], nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/selection/...
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/selection/
git commit -m "feat: add reviewer selection strategy"
```

---

### Task 6: Idempotency Helpers

**Files:**
- Create: `internal/pipeline/idempotency.go`
- Create: `internal/pipeline/idempotency_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/pipeline/idempotency_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/pipeline/... -run TestFormat -run TestParse
```

Expected: compilation error — `FormatNote` undefined.

- [ ] **Step 3: Implement idempotency helpers**

```go
// internal/pipeline/idempotency.go
// ABOUTME: formats and parses internal MR notes used to track processed pipeline runs
// ABOUTME: note format: "daylight:processed sha=<sha> files_hash=<hash>"
package pipeline

import (
	"fmt"
	"strings"
)

func FormatNote(sha, filesHash string) string {
	return fmt.Sprintf("daylight:processed sha=%s files_hash=%s", sha, filesHash)
}

func ParseNote(body string) (sha, filesHash string, ok bool) {
	if !strings.HasPrefix(body, "daylight:processed ") {
		return "", "", false
	}
	parts := strings.Fields(body)
	if len(parts) != 3 {
		return "", "", false
	}
	for _, part := range parts[1:] {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return "", "", false
		}
		switch kv[0] {
		case "sha":
			sha = kv[1]
		case "files_hash":
			filesHash = kv[1]
		}
	}
	return sha, filesHash, sha != "" && filesHash != ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/pipeline/... -run TestFormat -run TestParse
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/idempotency.go internal/pipeline/idempotency_test.go
git commit -m "feat: add idempotency note helpers"
```

---

### Task 7: GitLab Client

**Files:**
- Create: `internal/gitlab/client.go`
- Create: `internal/gitlab/client_test.go`

- [ ] **Step 1: Write the integration test (skipped by default)**

```go
// internal/gitlab/client_test.go
// ABOUTME: integration tests for the GitLab API client
// ABOUTME: skipped unless DAYLIGHT_GITLAB_TOKEN, DAYLIGHT_GITLAB_URL, TEST_PROJECT_ID, TEST_MR_IID are set
package gitlab_test

import (
	"context"
	"os"
	"testing"

	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipUnlessIntegration(t *testing.T) (token, url, projectID, mrIID string) {
	t.Helper()
	token = os.Getenv("DAYLIGHT_GITLAB_TOKEN")
	url = os.Getenv("DAYLIGHT_GITLAB_URL")
	projectID = os.Getenv("TEST_PROJECT_ID")
	mrIID = os.Getenv("TEST_MR_IID")
	if token == "" || url == "" || projectID == "" || mrIID == "" {
		t.Skip("set DAYLIGHT_GITLAB_TOKEN, DAYLIGHT_GITLAB_URL, TEST_PROJECT_ID, TEST_MR_IID to run")
	}
	return
}

func TestClient_MRNotes_Integration(t *testing.T) {
	token, url, projectID, mrIID := skipUnlessIntegration(t)

	client := gitlab.New(url, token)
	notes, err := client.MRNotes(context.Background(), projectID, mrIID)

	require.NoError(t, err)
	assert.NotNil(t, notes)
}

func TestClient_MRChanges_Integration(t *testing.T) {
	token, url, projectID, mrIID := skipUnlessIntegration(t)

	client := gitlab.New(url, token)
	files, author, err := client.MRChanges(context.Background(), projectID, mrIID)

	require.NoError(t, err)
	assert.NotEmpty(t, author)
	assert.NotNil(t, files)
}
```

- [ ] **Step 2: Implement the GitLab client**

```go
// internal/gitlab/client.go
// ABOUTME: GitLab REST API client for the five operations needed by the assignment pipeline
// ABOUTME: GitLabClient interface allows the pipeline to be tested with a mock
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Note struct {
	Body string `json:"body"`
}

type GitLabClient interface {
	MRNotes(ctx context.Context, projectID, mrIID string) ([]Note, error)
	// MRChanges returns the list of changed file paths and the MR author's username.
	MRChanges(ctx context.Context, projectID, mrIID string) (files []string, authorUsername string, err error)
	CODEOWNERSContent(ctx context.Context, projectID, ref string) (string, error)
	SetReviewers(ctx context.Context, projectID, mrIID string, usernames []string) error
	PostInternalNote(ctx context.Context, projectID, mrIID, body string) error
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) MRNotes(ctx context.Context, projectID, mrIID string) ([]Note, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s/notes?per_page=100", c.baseURL, projectID, mrIID)
	var notes []Note
	if err := c.get(ctx, url, &notes); err != nil {
		return nil, err
	}
	return notes, nil
}

type mrChangesResponse struct {
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
	Changes []struct {
		NewPath string `json:"new_path"`
	} `json:"changes"`
}

func (c *Client) MRChanges(ctx context.Context, projectID, mrIID string) ([]string, string, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s/changes", c.baseURL, projectID, mrIID)
	var resp mrChangesResponse
	if err := c.get(ctx, url, &resp); err != nil {
		return nil, "", err
	}
	files := make([]string, 0, len(resp.Changes))
	for _, ch := range resp.Changes {
		files = append(files, ch.NewPath)
	}
	return files, resp.Author.Username, nil
}

func (c *Client) CODEOWNERSContent(ctx context.Context, projectID, ref string) (string, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/CODEOWNERS/raw?ref=%s", c.baseURL, projectID, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitLab API status %d fetching CODEOWNERS", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

type userResponse struct {
	ID int `json:"id"`
}

func (c *Client) SetReviewers(ctx context.Context, projectID, mrIID string, usernames []string) error {
	ids := make([]int, 0, len(usernames))
	for _, username := range usernames {
		id, err := c.userID(ctx, username)
		if err != nil {
			return fmt.Errorf("lookup user %s: %w", username, err)
		}
		ids = append(ids, id)
	}
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s", c.baseURL, projectID, mrIID)
	return c.doJSON(ctx, http.MethodPut, url, map[string]any{"reviewer_ids": ids}, http.StatusOK)
}

func (c *Client) userID(ctx context.Context, username string) (int, error) {
	url := fmt.Sprintf("%s/api/v4/users?username=%s", c.baseURL, username)
	var users []userResponse
	if err := c.get(ctx, url, &users); err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, fmt.Errorf("user not found: %s", username)
	}
	return users[0].ID, nil
}

func (c *Client) PostInternalNote(ctx context.Context, projectID, mrIID, body string) error {
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s/notes", c.baseURL, projectID, mrIID)
	return c.doJSON(ctx, http.MethodPost, url,
		map[string]any{"body": body, "confidential": true},
		http.StatusCreated)
}

func (c *Client) get(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitLab API status %d for GET %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func (c *Client) doJSON(ctx context.Context, method, url string, payload any, expectedStatus int) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("GitLab API status %d for %s %s", resp.StatusCode, method, url)
	}
	return nil
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/gitlab/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/gitlab/
git commit -m "feat: add GitLab API client"
```

---

### Task 8: Pipeline

**Files:**
- Create: `internal/pipeline/pipeline.go`
- Modify: `internal/pipeline/pipeline_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/pipeline/pipeline_test.go
// ABOUTME: tests for the assignment pipeline using a hand-rolled mock GitLab client
// ABOUTME: one test per early-exit condition plus a happy path and author-exclusion test
package pipeline

import (
	"context"
	"testing"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/selection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	mrNotes           func(context.Context, string, string) ([]gitlab.Note, error)
	mrChanges         func(context.Context, string, string) ([]string, string, error)
	codeownersContent func(context.Context, string, string) (string, error)
	setReviewers      func(context.Context, string, string, []string) error
	postInternalNote  func(context.Context, string, string, string) error
}

func (m *mockClient) MRNotes(ctx context.Context, p, mr string) ([]gitlab.Note, error) {
	return m.mrNotes(ctx, p, mr)
}
func (m *mockClient) MRChanges(ctx context.Context, p, mr string) ([]string, string, error) {
	return m.mrChanges(ctx, p, mr)
}
func (m *mockClient) CODEOWNERSContent(ctx context.Context, p, ref string) (string, error) {
	return m.codeownersContent(ctx, p, ref)
}
func (m *mockClient) SetReviewers(ctx context.Context, p, mr string, u []string) error {
	return m.setReviewers(ctx, p, mr, u)
}
func (m *mockClient) PostInternalNote(ctx context.Context, p, mr, b string) error {
	return m.postInternalNote(ctx, p, mr, b)
}

func baseCfg() config.Config {
	return config.Config{ProjectID: "1", MRIID: "1", CommitSHA: "sha1", IsDraft: false}
}

func TestRun_Draft_ExitsEarly(t *testing.T) {
	cfg := baseCfg()
	cfg.IsDraft = true

	err := Run(context.Background(), cfg, nil, nil)

	require.NoError(t, err)
}

func TestRun_AlreadyProcessedBySHA_ExitsEarly(t *testing.T) {
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{{Body: FormatNote("sha1", "anyhash")}}, nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, nil)

	require.NoError(t, err)
}

func TestRun_AlreadyProcessedByFilesHash_ExitsEarly(t *testing.T) {
	files := []string{"/src/main.go"}
	hash := filesHash(files)
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{{Body: FormatNote("oldsha", hash)}}, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return files, "author", nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, nil)

	require.NoError(t, err)
}

func TestRun_NoOwners_Noop(t *testing.T) {
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return nil, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/src/main.go"}, "author", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "", nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, nil)

	require.NoError(t, err)
}

func TestRun_HappyPath_AssignsReviewerAndPostsNote(t *testing.T) {
	var assignedReviewers []string
	var postedNote string
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return nil, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/src/main.go"}, "author", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "[Backend][1]\n/src/ @alice @bob\n", nil
		},
		setReviewers: func(_ context.Context, _, _ string, u []string) error {
			assignedReviewers = u
			return nil
		},
		postInternalNote: func(_ context.Context, _, _, b string) error {
			postedNote = b
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, selection.RandomStrategy{})

	require.NoError(t, err)
	assert.Len(t, assignedReviewers, 1)
	assert.Contains(t, []string{"alice", "bob"}, assignedReviewers[0])
	assert.Contains(t, postedNote, "daylight:processed sha=sha1")
}

func TestRun_AuthorExcludedFromSelection(t *testing.T) {
	var assignedReviewers []string
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return nil, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/src/main.go"}, "alice", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "[Backend][1]\n/src/ @alice @bob\n", nil
		},
		setReviewers: func(_ context.Context, _, _ string, u []string) error {
			assignedReviewers = u
			return nil
		},
		postInternalNote: func(_ context.Context, _, _, _ string) error { return nil },
	}

	err := Run(context.Background(), baseCfg(), cl, selection.RandomStrategy{})

	require.NoError(t, err)
	assert.Equal(t, []string{"bob"}, assignedReviewers)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/pipeline/...
```

Expected: compilation error — `pipeline.Run` and `pipeline.filesHash` undefined.

- [ ] **Step 3: Implement the pipeline**

```go
// internal/pipeline/pipeline.go
// ABOUTME: orchestrates the reviewer assignment pipeline for a single merge request
// ABOUTME: checks idempotency, resolves ownership, selects reviewers, and records the result
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/ownership"
	"github.com/daylight-review/daylight/internal/selection"
)

func Run(ctx context.Context, cfg config.Config, gl gitlab.GitLabClient, strategy selection.SelectionStrategy) error {
	if cfg.IsDraft {
		return nil
	}

	notes, err := gl.MRNotes(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR notes: %w", err)
	}
	for _, note := range notes {
		sha, _, ok := ParseNote(note.Body)
		if ok && sha == cfg.CommitSHA {
			return nil
		}
	}

	files, author, err := gl.MRChanges(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR changes: %w", err)
	}
	hash := filesHash(files)

	for _, note := range notes {
		_, fh, ok := ParseNote(note.Body)
		if ok && fh == hash {
			return nil
		}
	}

	content, err := gl.CODEOWNERSContent(ctx, cfg.ProjectID, cfg.CommitSHA)
	if err != nil {
		return fmt.Errorf("fetch CODEOWNERS: %w", err)
	}

	sections := ownership.Parse(content)
	teamCandidates := ownership.Resolve(files, sections)
	if len(teamCandidates) == 0 {
		return nil
	}

	selected := make(map[string]bool)
	for _, candidates := range teamCandidates {
		reviewer, err := strategy.Select(candidates, author)
		if err != nil {
			continue
		}
		selected[reviewer] = true
	}
	if len(selected) == 0 {
		return nil
	}

	reviewers := make([]string, 0, len(selected))
	for r := range selected {
		reviewers = append(reviewers, r)
	}

	if err := gl.SetReviewers(ctx, cfg.ProjectID, cfg.MRIID, reviewers); err != nil {
		return fmt.Errorf("set reviewers: %w", err)
	}

	if err := gl.PostInternalNote(ctx, cfg.ProjectID, cfg.MRIID, FormatNote(cfg.CommitSHA, hash)); err != nil {
		return fmt.Errorf("post note: %w", err)
	}

	return nil
}

func filesHash(files []string) string {
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/pipeline/...
```

Expected: all tests PASS.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go
git commit -m "feat: add assignment pipeline"
```

---

### Task 9: main.go

**Files:**
- Create: `cmd/daylight/main.go`

- [ ] **Step 1: Implement main.go**

```go
// cmd/daylight/main.go
// ABOUTME: entry point for the daylight CLI — wires config, GitLab client, and pipeline
// ABOUTME: exits with code 1 and prints the error on any failure
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/pipeline"
	"github.com/daylight-review/daylight/internal/selection"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "assign" {
		fmt.Fprintln(os.Stderr, "usage: daylight assign")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client := gitlab.New(cfg.GitLabURL, cfg.Token)

	if err := pipeline.Run(context.Background(), cfg, client, selection.RandomStrategy{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build the binary**

```bash
go build -o daylight ./cmd/daylight
```

Expected: `./daylight` binary created, no errors.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/daylight/main.go
git commit -m "feat: add CLI entry point"
```

---

### Task 10: Dockerfile + CI Template

**Files:**
- Create: `Dockerfile`
- Create: `.gitlab-ci.yml`

- [ ] **Step 1: Create Dockerfile**

```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o daylight ./cmd/daylight

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /app/daylight /usr/local/bin/daylight
ENTRYPOINT ["daylight"]
```

- [ ] **Step 2: Create CI template**

```yaml
# .gitlab-ci.yml
# Example CI template for Daylight automatic reviewer assignment.
# Add DAYLIGHT_GITLAB_TOKEN as a protected CI/CD variable in group settings.
daylight-assign:
  image: ghcr.io/daylight-review/daylight:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    DAYLIGHT_GITLAB_TOKEN: $DAYLIGHT_GITLAB_TOKEN
  script:
    - daylight assign
```

- [ ] **Step 3: Verify Docker build**

```bash
docker build -t daylight:local .
```

Expected: image builds successfully, no errors.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile .gitlab-ci.yml
git commit -m "feat: add Dockerfile and CI template"
```
