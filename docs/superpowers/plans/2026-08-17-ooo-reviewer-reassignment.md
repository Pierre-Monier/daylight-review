# Out-of-office Reviewer Reassignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a human flag an assigned reviewer as out-of-office via an MR comment, so Daylight replaces that reviewer on the next job run.

**Architecture:** A human posts a plain MR comment `daylight:ooo @alice`. Daylight already fetches all MR notes each run; on the next run (push or job retry) it parses OOO commands from the notes, excludes flagged users from selection, backfills their scopes, records the flagged set in the internal note (sticky for the MR), and posts a visible warning when a scope has no available replacement.

**Tech Stack:** Go, standard library only, `testify` for assertions. Pure unit tests in `internal/pipeline` driving a hand-rolled mock GitLab client. No network.

## Global Constraints

- Every source file starts with two `// ABOUTME:` comment lines describing what it does.
- TDD: write the failing test first, watch it fail, then the minimal implementation.
- Commit after each task once its tests are green.
- Names describe domain purpose, not implementation or history.
- Reference spec: `docs/superpowers/specs/2026-08-17-ooo-reviewer-reassignment-design.md`.
- Command syntax is exactly `daylight:ooo @alice`; the `@` is optional; one username per command.
- The internal note prefix stays `daylight:processed `; the OOO warning note is **non-confidential**.
- Build check: `go build ./...`; test check: `go test ./internal/pipeline`.

---

### Task 1: Parse OOO commands from note bodies

**Files:**
- Create: `internal/pipeline/ooo.go`
- Test: `internal/pipeline/ooo_test.go`

**Interfaces:**
- Consumes: `gitlab.Note` (existing, has `Body string`).
- Produces:
  - `const oooCommandPrefix = "daylight:ooo"`
  - `func ParseOOOCommand(body string) (username string, ok bool)` — parses one command note; `@` optional; false for anything not an OOO command.
  - `func requestedOOO(notes []gitlab.Note) []string` — sorted, deduped usernames from all OOO command notes.
  - `func unionOOO(a, b []string) []string` — sorted, deduped union.

- [ ] **Step 1: Write the failing test**

Create `internal/pipeline/ooo_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline -run 'ParseOOOCommand|RequestedOOO|UnionOOO' -v`
Expected: FAIL — `undefined: ParseOOOCommand` (compile error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/pipeline/ooo.go`:

```go
// ABOUTME: parses out-of-office command notes and builds the set of flagged usernames
// ABOUTME: command syntax is "daylight:ooo @username" (the @ is optional)
package pipeline

import (
	"sort"
	"strings"

	"github.com/daylight-review/daylight/internal/gitlab"
)

const oooCommandPrefix = "daylight:ooo"

// ParseOOOCommand extracts the username from a "daylight:ooo @username" note body.
// The leading @ is optional. ok is false for any body that is not an OOO command.
func ParseOOOCommand(body string) (username string, ok bool) {
	fields := strings.Fields(body)
	if len(fields) < 2 || fields[0] != oooCommandPrefix {
		return "", false
	}
	return strings.TrimPrefix(fields[1], "@"), true
}

// requestedOOO returns the sorted, deduped usernames flagged out-of-office across all notes.
func requestedOOO(notes []gitlab.Note) []string {
	set := make(map[string]bool)
	for _, n := range notes {
		if u, ok := ParseOOOCommand(n.Body); ok {
			set[u] = true
		}
	}
	return sortedKeys(set)
}

// unionOOO returns the sorted, deduped union of two username lists.
func unionOOO(a, b []string) []string {
	set := make(map[string]bool, len(a)+len(b))
	for _, u := range a {
		set[u] = true
	}
	for _, u := range b {
		set[u] = true
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pipeline -run 'ParseOOOCommand|RequestedOOO|UnionOOO' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/ooo.go internal/pipeline/ooo_test.go
git commit -m "feat: parse daylight:ooo command notes"
```

---

### Task 2: Store the OOO set in the internal note

**Files:**
- Modify: `internal/pipeline/idempotency.go`
- Modify: `internal/pipeline/pipeline.go:33` (ParseNote call), `internal/pipeline/pipeline.go:74` (FormatNote call)
- Modify: `internal/pipeline/idempotency_test.go`, `internal/pipeline/pipeline_test.go` (existing FormatNote/ParseNote call sites)

**Interfaces:**
- Consumes: nothing new.
- Produces (changed signatures):
  - `func FormatNote(sha string, assignments map[string][]string, ooo []string) string`
  - `func ParseNote(body string) (sha string, assignments map[string][]string, ooo []string, ok bool)`

This task changes two exported signatures, so every caller in the package must be updated in the same commit to keep the package compiling.

- [ ] **Step 1: Write the failing test**

Add to `internal/pipeline/idempotency_test.go`:

```go
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
```

(If `idempotency_test.go` lacks the `testify/assert` import, add it.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline -run 'FormatParseNote|LegacyNote' -v`
Expected: FAIL — too many return values / too few arguments (compile error).

- [ ] **Step 3: Write minimal implementation**

Edit `internal/pipeline/idempotency.go` — extend `noteState` and both functions:

```go
type noteState struct {
	SHA         string              `json:"sha"`
	Assignments map[string][]string `json:"assignments"`
	Ooo         []string            `json:"ooo,omitempty"`
}

// FormatNote encodes the processed commit SHA, the scope→reviewers map, and the
// out-of-office usernames as a note body.
func FormatNote(sha string, assignments map[string][]string, ooo []string) string {
	data, _ := json.Marshal(noteState{SHA: sha, Assignments: assignments, Ooo: ooo})
	return notePrefix + string(data)
}

// ParseNote decodes a note body produced by FormatNote. ok is false for any body that is
// not a daylight note or whose SHA is missing. Notes written before OOO support decode
// with an empty ooo list.
func ParseNote(body string) (sha string, assignments map[string][]string, ooo []string, ok bool) {
	if !strings.HasPrefix(body, notePrefix) {
		return "", nil, nil, false
	}
	var state noteState
	if err := json.Unmarshal([]byte(body[len(notePrefix):]), &state); err != nil {
		return "", nil, nil, false
	}
	if state.SHA == "" {
		return "", nil, nil, false
	}
	return state.SHA, state.Assignments, state.Ooo, true
}
```

- [ ] **Step 4: Update the two non-test call sites in `pipeline.go`**

At `internal/pipeline/pipeline.go` line ~32-37, update the ParseNote loop and add a `prevOoo` variable:

```go
	var prevSHA string
	var prevAssignments map[string][]string
	var prevOoo []string
	// Notes are newest-first, so the first daylight note is the current state.
	for _, note := range notes {
		if sha, assignments, ooo, ok := ParseNote(note.Body); ok {
			prevSHA, prevAssignments, prevOoo = sha, assignments, ooo
			break
		}
	}
```

At `internal/pipeline/pipeline.go` line ~74, update the FormatNote call to pass `prevOoo` for now (Task 5 replaces `prevOoo` with the merged set):

```go
	if err := gl.PostNote(ctx, cfg.ProjectID, cfg.MRIID, FormatNote(cfg.CommitSHA, assignments, prevOoo), true); err != nil {
```

`prevOoo` is now referenced, so it compiles. `prevAssignments` usage is unchanged.

- [ ] **Step 5: Update existing test call sites**

In `internal/pipeline/pipeline_test.go` and `internal/pipeline/idempotency_test.go`, update every `FormatNote(...)` and `ParseNote(...)` call to the new signatures:
- `FormatNote("sha1", map[string][]string{...})` → `FormatNote("sha1", map[string][]string{...}, nil)`
- `sha, a, ok := ParseNote(body)` → `sha, a, _, ok := ParseNote(body)` (add the ooo return, discard with `_` where the test doesn't assert on it).

Search to find them all:

```bash
grep -rn "FormatNote(\|ParseNote(" internal/pipeline/*_test.go
```

Update each. For `findProcessedNote` in `pipeline_test.go`, the `ParseNote` call becomes `if _, _, _, ok := ParseNote(n.body); ok {`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/pipeline`
Expected: PASS (all existing tests plus the two new ones).

- [ ] **Step 7: Commit**

```bash
git add internal/pipeline/idempotency.go internal/pipeline/idempotency_test.go internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go
git commit -m "feat: store out-of-office set in the processed note"
```

---

### Task 3: Exclude OOO reviewers and backfill in DiffAndSelect

**Files:**
- Modify: `internal/pipeline/pipeline.go` (`DiffAndSelect`)
- Modify: `cmd/daylight/main.go:108` (call site)
- Modify: `internal/pipeline/pipeline.go:55` (call site in `Run`)
- Modify: `internal/pipeline/pipeline_test.go` (existing DiffAndSelect call sites + new tests)

**Interfaces:**
- Consumes: `ooo []string` (from Task 1's union).
- Produces (changed signature + new type):
  - `type Unfilled struct { Scope string; Username string }`
  - `func DiffAndSelect(files []string, sections []ownership.Section, author string, previous map[string][]string, ooo []string, strategy selection.SelectionStrategy, printf func(string, ...any)) (map[string][]string, []Unfilled)`
  - A kept scope drops any OOO reviewer and backfills one replacement each (excluding OOO + author + already-assigned); when no replacement exists, the dropped reviewer is added to the returned `[]Unfilled` and the scope is left short.

- [ ] **Step 1: Write the failing test**

Add to `internal/pipeline/pipeline_test.go`:

```go
func TestDiffAndSelect_OOOReviewer_Replaced(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice @bob\n")
	files := []string{"/src/main.go"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, unfilled := DiffAndSelect(files, sections, "", previous, []string{"alice"}, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"bob"}, got["Backend"])
	assert.Empty(t, unfilled)
}

func TestDiffAndSelect_OOOReviewer_StickyAcrossScopes(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice @bob\n[Frontend][1]\n/web/ @alice @dave\n")
	files := []string{"/src/main.go", "/web/app.js"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, unfilled := DiffAndSelect(files, sections, "", previous, []string{"alice"}, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"bob"}, got["Backend"])
	assert.Equal(t, []string{"dave"}, got["Frontend"])
	assert.Empty(t, unfilled)
}

func TestDiffAndSelect_OOOReviewer_NoReplacement_Unfilled(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n")
	files := []string{"/src/main.go"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, unfilled := DiffAndSelect(files, sections, "", previous, []string{"alice"}, firstStrategy{}, noopPrintf)

	assert.NotContains(t, got, "Backend")
	assert.Equal(t, []Unfilled{{Scope: "Backend", Username: "alice"}}, unfilled)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline -run 'DiffAndSelect_OOO' -v`
Expected: FAIL — `DiffAndSelect` returns 1 value, not 2 (compile error).

- [ ] **Step 3: Rewrite `DiffAndSelect`**

Replace the `DiffAndSelect` function in `internal/pipeline/pipeline.go` (keep the file's other functions unchanged). Add the `Unfilled` type just above it:

```go
// Unfilled names a scope left without a reviewer because an out-of-office reviewer
// had no eligible replacement.
type Unfilled struct {
	Scope    string
	Username string
}

// DiffAndSelect computes the new per-scope reviewer assignments. Scopes matched by the
// changed files are diffed against the previously-stored assignments: a scope present in
// both keeps its stored reviewers verbatim, except out-of-office reviewers are dropped and
// each is replaced by a freshly-selected owner; a newly-matched scope gets freshly-selected
// reviewers (count from the section, excluding the author, out-of-office users, and anyone
// already assigned); a scope no longer matched is dropped. When an out-of-office reviewer
// cannot be replaced, the scope is left short and the reviewer is returned in unfilled.
func DiffAndSelect(files []string, sections []ownership.Section, author string, previous map[string][]string, ooo []string, strategy selection.SelectionStrategy, printf func(string, ...any)) (map[string][]string, []Unfilled) {
	printf("=== FILE MATCHING ===")
	printf("author: %q (excluded from selection)", author)
	printf("changed files (%d):", len(files))
	for _, f := range files {
		normalized := ownership.NormalizePath(f)
		matched := false
		for _, s := range sections {
			for _, r := range s.Rules {
				if strings.HasPrefix(normalized, r.Pattern) {
					printf("  %-40s → [%s] rule %s", f, s.Name, r.Pattern)
					matched = true
				}
			}
		}
		if !matched {
			printf("  %-40s → (no match)", f)
		}
	}

	currentPools := ownership.Resolve(files, sections)

	sectionCount := make(map[string]int, len(sections))
	for _, s := range sections {
		sectionCount[s.Name] = s.RequiredCount
	}

	oooSet := make(map[string]bool, len(ooo))
	for _, u := range ooo {
		oooSet[u] = true
	}

	result := make(map[string][]string)
	assigned := make(map[string]bool)
	for u := range oooSet {
		assigned[u] = true // out-of-office reviewers are never selected
	}
	var unfilled []Unfilled

	printf("=== SCOPE DIFF ===")
	for scope := range currentPools {
		prev, ok := previous[scope]
		if !ok {
			continue
		}
		kept := make([]string, 0, len(prev))
		for _, r := range prev {
			if oooSet[r] {
				printf("[%s] reviewer %s is out, selecting replacement", scope, r)
				replacement := selectN(currentPools[scope], 1, author, assigned, strategy, printf)
				if len(replacement) == 0 {
					printf("[%s] no replacement available for %s", scope, r)
					unfilled = append(unfilled, Unfilled{Scope: scope, Username: r})
					continue
				}
				kept = append(kept, replacement[0])
				continue
			}
			kept = append(kept, r)
			assigned[r] = true
		}
		if len(kept) > 0 {
			result[scope] = kept
		}
		printf("[%s] kept: %s", scope, strings.Join(kept, ", "))
	}
	for scope, prev := range previous {
		if _, ok := currentPools[scope]; !ok {
			printf("[%s] removed: %s", scope, strings.Join(prev, ", "))
		}
	}
	for scope, candidates := range currentPools {
		if _, ok := previous[scope]; ok {
			continue
		}
		count := sectionCount[scope]
		if count == 0 {
			count = 1
		}
		printf("[%s] new — pool: %s, need: %d", scope, strings.Join(candidates, ", "), count)
		if selected := selectN(candidates, count, author, assigned, strategy, printf); len(selected) > 0 {
			result[scope] = selected
		}
	}

	return result, unfilled
}
```

`selectN` marks each selected reviewer in `assigned`, so replacements are automatically excluded from later scopes — no manual bookkeeping needed for the replacement.

- [ ] **Step 4: Update the two non-test call sites**

`internal/pipeline/pipeline.go` line ~55 in `Run`:

```go
	assignments, _ := DiffAndSelect(files, sections, author, prevAssignments, prevOoo, strategy, log.Printf)
```

(Task 5 replaces `prevOoo` with the merged set and consumes the second return value; for now discard it with `_`.)

`cmd/daylight/main.go` line ~108:

```go
	assignments, _ := pipeline.DiffAndSelect(changedFiles, sections, *author, nil, nil, selection.RandomStrategy{}, printf)
```

- [ ] **Step 5: Update existing DiffAndSelect test call sites**

In `internal/pipeline/pipeline_test.go`, every existing `got := DiffAndSelect(...)` call needs the new `ooo` argument (`nil`) and the second return value. Find them:

```bash
grep -n "DiffAndSelect(" internal/pipeline/pipeline_test.go
```

For each existing call, change:
- `got := DiffAndSelect(files, sections, "", previous, firstStrategy{}, noopPrintf)`
- to `got, _ := DiffAndSelect(files, sections, "", previous, nil, firstStrategy{}, noopPrintf)`

(Insert `nil` before `firstStrategy{}` and add `, _` to the assignment. Do this for all of: `TestDiffAndSelect_PresentScope_KeptVerbatim`, `_NewScopeAdded_OnlyNewSelected`, `_ScopeRemoved_Dropped`, `_AllScopesRemoved_Empty`, `_NewScope_ExcludesAlreadyAssigned`, `_NewScope_ExcludesAuthor`, `_NewScope_PoolExhausted_NoReviewer`.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/pipeline`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go cmd/daylight/main.go
git commit -m "feat: replace out-of-office reviewers in DiffAndSelect"
```

---

### Task 4: Format the no-replacement warning note

**Files:**
- Modify: `internal/pipeline/ooo.go`
- Modify: `internal/pipeline/ooo_test.go`

**Interfaces:**
- Produces: `func oooWarningNote(scope, username string) (body string, confidential bool)` — non-confidential warning naming the out person and the scope.

- [ ] **Step 1: Write the failing test**

Add to `internal/pipeline/ooo_test.go`:

```go
func TestOOOWarningNote(t *testing.T) {
	body, confidential := oooWarningNote("Backend", "alice")
	assert.False(t, confidential)
	assert.Contains(t, body, "@alice")
	assert.Contains(t, body, "Backend")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pipeline -run 'OOOWarningNote' -v`
Expected: FAIL — `undefined: oooWarningNote`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/pipeline/ooo.go` (add `"fmt"` to the imports):

```go
// oooWarningNote returns the body and visibility of the visible note posted when an
// out-of-office reviewer has no eligible replacement in their scope.
func oooWarningNote(scope, username string) (body string, confidential bool) {
	body = fmt.Sprintf("⚠️ @%s is out and no other owner of [%s] is available — please assign manually.", username, scope)
	return body, false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pipeline -run 'OOOWarningNote' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/ooo.go internal/pipeline/ooo_test.go
git commit -m "feat: format out-of-office no-replacement warning note"
```

---

### Task 5: Wire OOO handling into the pipeline run

**Files:**
- Modify: `internal/pipeline/pipeline.go` (`Run`)
- Modify: `internal/pipeline/pipeline_test.go` (new integration tests)

**Interfaces:**
- Consumes: `requestedOOO`, `unionOOO`, `oooWarningNote` (Tasks 1 & 4), the two-value `DiffAndSelect` (Task 3), the OOO-aware `FormatNote`/`ParseNote` (Task 2).
- Produces: `Run` now (a) overrides the same-SHA skip when a new OOO command is present, (b) passes the merged OOO set to `DiffAndSelect`, (c) stores the merged OOO set in the note, (d) posts each no-replacement warning once.

- [ ] **Step 1: Write the failing test**

Add to `internal/pipeline/pipeline_test.go`:

```go
func TestRun_OOOCommand_SameSHA_Reassigns(t *testing.T) {
	var assignedReviewers []string
	var posted []recordedNote
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{
				{Body: FormatNote("sha1", map[string][]string{"Backend": {"alice"}}, nil)},
				{Body: "daylight:ooo @alice"},
			}, nil
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
		postNote: func(_ context.Context, _, _, b string, c bool) error {
			posted = append(posted, recordedNote{b, c})
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.Equal(t, []string{"bob"}, assignedReviewers)
	body, ok := findProcessedNote(posted)
	require.True(t, ok)
	_, postedAssignments, postedOoo, ok := ParseNote(body)
	require.True(t, ok)
	assert.Equal(t, []string{"bob"}, postedAssignments["Backend"])
	assert.Equal(t, []string{"alice"}, postedOoo)
}

func TestRun_OOOAlreadyRecorded_SameSHA_Skips(t *testing.T) {
	setCalled := false
	postCalled := false
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{
				{Body: FormatNote("sha1", map[string][]string{"Backend": {"bob"}}, []string{"alice"})},
				{Body: "daylight:ooo @alice"},
			}, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/src/main.go"}, "author", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "[Backend][1]\n/src/ @alice @bob\n", nil
		},
		setReviewers: func(_ context.Context, _, _ string, _ []string) error {
			setCalled = true
			return nil
		},
		postNote: func(_ context.Context, _, _, _ string, _ bool) error {
			postCalled = true
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.False(t, setCalled, "OOO already recorded at this SHA, must skip")
	assert.False(t, postCalled, "must not post a new note when skipping")
}

func TestRun_OOONoReplacement_PostsWarningOnce(t *testing.T) {
	var posted []recordedNote
	warningBody, _ := oooWarningNote("Backend", "alice")
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{
				{Body: FormatNote("sha1", map[string][]string{"Backend": {"alice"}}, nil)},
				{Body: "daylight:ooo @alice"},
			}, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/src/main.go"}, "author", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "[Backend][1]\n/src/ @alice\n", nil
		},
		setReviewers: func(_ context.Context, _, _ string, _ []string) error { return nil },
		postNote: func(_ context.Context, _, _, b string, c bool) error {
			posted = append(posted, recordedNote{b, c})
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.Contains(t, posted, recordedNote{warningBody, false})
}

func TestRun_OOOWarningAlreadyPosted_NotReposted(t *testing.T) {
	var posted []recordedNote
	warningBody, _ := oooWarningNote("Backend", "alice")
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{
				{Body: FormatNote("oldsha", map[string][]string{"Backend": {"alice"}}, nil)},
				{Body: "daylight:ooo @alice"},
				{Body: warningBody},
			}, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/src/main.go"}, "author", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "[Backend][1]\n/src/ @alice\n", nil
		},
		setReviewers: func(_ context.Context, _, _ string, _ []string) error { return nil },
		postNote: func(_ context.Context, _, _, b string, c bool) error {
			posted = append(posted, recordedNote{b, c})
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.NotContains(t, posted, recordedNote{warningBody, false})
}
```

Note: `TestRun_OOOWarningAlreadyPosted_NotReposted` uses `oldsha` (different from `sha1`) so the run proceeds via the normal new-commit path, then recomputes the same warning and must suppress it because it already exists in the notes.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/pipeline -run 'TestRun_OOO' -v`
Expected: FAIL — reassignment/skip/warning behavior not yet implemented (e.g. `TestRun_OOOCommand_SameSHA_Reassigns` fails because the run skips on matching SHA).

- [ ] **Step 3: Implement the wiring in `Run`**

Edit `internal/pipeline/pipeline.go`. Replace the skip check (currently `if prevSHA == cfg.CommitSHA { ... }`) and the assignment/note-posting section with OOO-aware logic.

After the ParseNote loop that sets `prevSHA, prevAssignments, prevOoo`, compute the merged OOO set and change the skip condition:

```go
	ooo := unionOOO(prevOoo, requestedOOO(notes))
	if prevSHA == cfg.CommitSHA && len(ooo) == len(prevOoo) {
		log.Println("already processed this commit, skipping")
		return nil
	}
```

(`ooo` is the sorted union and always a superset of `prevOoo`, so equal lengths mean no new OOO command — safe to skip.)

Change the `DiffAndSelect` call to pass `ooo` and capture the unfilled scopes:

```go
	assignments, unfilled := DiffAndSelect(files, sections, author, prevAssignments, ooo, strategy, log.Printf)
```

Change the note-posting call to store the merged `ooo`:

```go
	if err := gl.PostNote(ctx, cfg.ProjectID, cfg.MRIID, FormatNote(cfg.CommitSHA, assignments, ooo), true); err != nil {
		return fmt.Errorf("post note: %w", err)
	}
```

After that `PostNote` (and before the feedback-note block), post each no-replacement warning once:

```go
	for _, u := range unfilled {
		body, confidential := oooWarningNote(u.Scope, u.Username)
		if noteExists(notes, body) {
			continue
		}
		if err := gl.PostNote(ctx, cfg.ProjectID, cfg.MRIID, body, confidential); err != nil {
			log.Printf("warning: post out-of-office warning: %v", err)
		}
	}
```

Add the helper near the bottom of `pipeline.go`:

```go
// noteExists reports whether a note with exactly the given body is already on the MR.
func noteExists(notes []gitlab.Note, body string) bool {
	for _, n := range notes {
		if n.Body == body {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/pipeline`
Expected: PASS (all four new `TestRun_OOO*` tests plus the full existing suite).

- [ ] **Step 5: Run the whole test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go
git commit -m "feat: reassign out-of-office reviewers in pipeline run"
```

---

### Task 6: Document the feature

**Files:**
- Modify: `CLAUDE.md` (Architecture → Idempotency section, and remove/replace the stale "Work in progress" note if it still references feedback-note)

**Interfaces:** none (docs only).

- [ ] **Step 1: Update CLAUDE.md**

In the "Idempotency & per-scope stability" section of `CLAUDE.md`, add a short paragraph describing OOO reassignment:

```markdown
A reviewer can be flagged out-of-office with an MR comment `daylight:ooo @user`. On the
next run (push or job retry) Daylight reads the comment from the notes it already fetches,
drops that reviewer from their scope, and backfills a replacement (excluding the author,
out-of-office users, and already-assigned reviewers). The flagged set is stored in the
processed note (`ooo`) and is sticky for the MR — a flagged user is never re-selected. A
new OOO command overrides the same-SHA skip. If a scope has no available replacement it is
left short and a one-time non-confidential warning note is posted.
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: describe out-of-office reviewer reassignment"
```

---

## Self-Review

**Spec coverage:**
- Command syntax `daylight:ooo @alice` → Task 1 (`ParseOOOCommand`). ✓
- Trigger via notes read on next run → Task 5 (Run reads notes, no webhook). ✓
- Idempotency override on new OOO command → Task 5 (skip condition). ✓
- OOO list stored in note, additive, sticky → Task 2 (`Ooo` field) + Task 5 (union stored). ✓
- Kept-scope drop + backfill; new-scope exclusion → Task 3 (`DiffAndSelect`). ✓
- No-replacement → drop + `Unfilled` + visible warning → Task 3 (`Unfilled`) + Task 4 (`oooWarningNote`) + Task 5 (post once). ✓
- Warning posted once → Task 5 (`noteExists`). ✓
- Testing cases from spec → covered across Tasks 1–5. ✓
- Out of scope (external source, un-OOO, scheduled polling) → not implemented. ✓

**Placeholder scan:** No TBD/TODO; every code and test step contains complete code.

**Type consistency:**
- `FormatNote(sha, assignments, ooo)` / `ParseNote → (sha, assignments, ooo, ok)` used consistently in Tasks 2 & 5.
- `DiffAndSelect(..., ooo, strategy, printf) → (map, []Unfilled)` used consistently in Tasks 3 & 5 and both non-test callers.
- `Unfilled{Scope, Username}` fields consistent between definition (Task 3) and use (Task 5, `u.Scope`/`u.Username`).
- `oooWarningNote(scope, username) → (body, confidential)` consistent between Task 4 and Task 5.
- `requestedOOO`, `unionOOO`, `noteExists` signatures consistent between definition and use.
