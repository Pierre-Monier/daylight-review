# Per-scope Reviewer Stability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the reviewer-assignment pipeline stable per CODEOWNERS scope — keep existing reviewers for scopes that stay matched, select reviewers only for newly-matched scopes, and drop reviewers for scopes that stop matching.

**Architecture:** The internal MR note becomes the single source of truth, storing a scope→reviewers map (JSON). Each run diffs the stored scopes against the currently-matched scopes: present scopes keep their reviewers verbatim, new scopes get freshly-selected reviewers, removed scopes are dropped. `SetReviewers` is called only when the reviewer set actually changes.

**Tech Stack:** Go, standard library (`encoding/json`), testify for assertions.

---

## Sequencing note

Go compiles per package. Changing `FormatNote`/`ParseNote`'s signature in `internal/pipeline` forces `Run` and every test that calls them to change in the **same commit**. The task order below keeps the `pipeline` package compiling and green after every commit:

1. **Task 1** is purely additive — new functions `DiffAndSelect`, `Reviewers`, `selectN`, plus test helpers. Nothing existing changes.
2. **Task 2** migrates the `check` CLI subcommand onto the new functions. Only `main.go` changes.
3. **Task 3** is the atomic note-format + `Run` rewire: it changes `FormatNote`/`ParseNote`, rewires `Run`, removes the now-unused `ResolveAndSelect`/`filesHash`, and updates all affected tests in one commit.

## File structure

- `internal/pipeline/pipeline.go` — gains `DiffAndSelect`, `selectN`, `Reviewers`, `equalStrings`; loses `ResolveAndSelect`, `filesHash`; `Run` rewired.
- `internal/pipeline/pipeline_test.go` — gains diff tests + `firstStrategy`/`noopPrintf` helpers; Run tests updated.
- `internal/pipeline/idempotency.go` — `FormatNote`/`ParseNote` become JSON state (sha + assignments).
- `internal/pipeline/idempotency_test.go` — round-trip tests updated for the new signature.
- `cmd/daylight/main.go` — `runCheck` uses `DiffAndSelect` + `Reviewers`.

---

## Task 1: Add the scope-diff core (additive)

**Files:**
- Modify: `internal/pipeline/pipeline.go` (add functions, remove nothing)
- Test: `internal/pipeline/pipeline_test.go` (add helpers + diff tests)

- [ ] **Step 1: Add test helpers and the first diff test**

Append to `internal/pipeline/pipeline_test.go`. Add the imports `"github.com/daylight-review/daylight/internal/ownership"` to the existing import block (keep existing imports).

```go
// firstStrategy deterministically picks the first eligible candidate, so diff tests
// can assert exact reviewers. Pools with more than one owner come from ownership.Resolve
// in non-deterministic map order, so exact-reviewer assertions use single-owner pools or
// pools reduced to one eligible candidate by exclusion.
type firstStrategy struct{}

func (firstStrategy) Select(candidates []string, exclude string) (string, error) {
	for _, c := range candidates {
		if c != exclude {
			return c, nil
		}
	}
	return "", selection.ErrNoEligibleReviewer
}

func noopPrintf(string, ...any) {}

func TestDiffAndSelect_PresentScope_KeptVerbatim(t *testing.T) {
	sections := ownership.Parse("[Backend][2]\n/src/ @alice @bob @carol\n")
	files := []string{"/src/main.go"}
	previous := map[string][]string{"Backend": {"alice", "bob"}}

	got := DiffAndSelect(files, sections, "", previous, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"alice", "bob"}, got["Backend"])
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pipeline/ -run TestDiffAndSelect_PresentScope_KeptVerbatim -v`
Expected: compile error — `undefined: DiffAndSelect`.

- [ ] **Step 3: Implement `DiffAndSelect`, `selectN`, `Reviewers`**

Add to `internal/pipeline/pipeline.go` (keep all existing code for now). Confirm the import block already has `"strings"` (it does).

```go
// DiffAndSelect computes the new per-scope reviewer assignments. Scopes matched by the
// changed files are diffed against the previously-stored assignments: a scope present in
// both keeps its stored reviewers verbatim; a newly-matched scope gets freshly-selected
// reviewers (count from the section, excluding the author and anyone already assigned);
// a scope no longer matched is dropped. Emits per-step detail via printf.
func DiffAndSelect(files []string, sections []ownership.Section, author string, previous map[string][]string, strategy selection.SelectionStrategy, printf func(string, ...any)) map[string][]string {
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

	result := make(map[string][]string)
	assigned := make(map[string]bool)

	printf("=== SCOPE DIFF ===")
	for scope := range currentPools {
		if prev, ok := previous[scope]; ok {
			result[scope] = prev
			for _, r := range prev {
				assigned[r] = true
			}
			printf("[%s] kept: %s", scope, strings.Join(prev, ", "))
		}
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

	return result
}

// selectN selects up to count reviewers from candidates, excluding the author and anyone
// already in the assigned set. Selected reviewers are added to assigned so later scopes
// don't reuse them. Stops early (with fewer reviewers) if the pool is exhausted.
func selectN(candidates []string, count int, author string, assigned map[string]bool, strategy selection.SelectionStrategy, printf func(string, ...any)) []string {
	selected := make([]string, 0, count)
	for i := 0; i < count; i++ {
		remaining := make([]string, 0, len(candidates))
		for _, c := range candidates {
			if !assigned[c] {
				remaining = append(remaining, c)
			}
		}
		reviewer, err := strategy.Select(remaining, author)
		if err != nil {
			printf("  round %d: pool exhausted, no reviewer assigned", i+1)
			break
		}
		assigned[reviewer] = true
		selected = append(selected, reviewer)
		printf("  round %d: selected %s", i+1, reviewer)
	}
	return selected
}

// Reviewers flattens a scope→reviewers map into a sorted, de-duplicated reviewer list.
func Reviewers(assignments map[string][]string) []string {
	set := make(map[string]bool)
	for _, reviewers := range assignments {
		for _, r := range reviewers {
			set[r] = true
		}
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/pipeline/ -run TestDiffAndSelect_PresentScope_KeptVerbatim -v`
Expected: PASS. (Existing tests still compile — nothing was removed.)

- [ ] **Step 5: Add the remaining diff tests**

Append to `internal/pipeline/pipeline_test.go`:

```go
func TestDiffAndSelect_NewScopeAdded_OnlyNewSelected(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n[Frontend][1]\n/web/ @carol\n")
	files := []string{"/src/main.go", "/web/app.js"}
	previous := map[string][]string{"Backend": {"alice"}}

	got := DiffAndSelect(files, sections, "", previous, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"alice"}, got["Backend"])
	assert.Equal(t, []string{"carol"}, got["Frontend"])
}

func TestDiffAndSelect_ScopeRemoved_Dropped(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n[Frontend][1]\n/web/ @carol\n")
	files := []string{"/src/main.go"}
	previous := map[string][]string{"Backend": {"alice"}, "Frontend": {"carol"}}

	got := DiffAndSelect(files, sections, "", previous, firstStrategy{}, noopPrintf)

	assert.Equal(t, map[string][]string{"Backend": {"alice"}}, got)
}

func TestDiffAndSelect_AllScopesRemoved_Empty(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n")
	files := []string{"/docs/readme.md"}
	previous := map[string][]string{"Backend": {"alice"}}

	got := DiffAndSelect(files, sections, "", previous, firstStrategy{}, noopPrintf)

	assert.Empty(t, got)
}

func TestDiffAndSelect_NewScope_ExcludesAlreadyAssigned(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n[Frontend][1]\n/web/ @alice @dave\n")
	files := []string{"/src/main.go", "/web/app.js"}
	previous := map[string][]string{"Backend": {"alice"}}

	got := DiffAndSelect(files, sections, "", previous, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"alice"}, got["Backend"])
	assert.Equal(t, []string{"dave"}, got["Frontend"])
}

func TestDiffAndSelect_NewScope_ExcludesAuthor(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice @bob\n")
	files := []string{"/src/main.go"}

	got := DiffAndSelect(files, sections, "alice", nil, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"bob"}, got["Backend"])
}

func TestDiffAndSelect_NewScope_PoolExhausted_NoReviewer(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n")
	files := []string{"/src/main.go"}

	got := DiffAndSelect(files, sections, "alice", nil, firstStrategy{}, noopPrintf)

	_, has := got["Backend"]
	assert.False(t, has)
}

func TestReviewers_SortedDeduped(t *testing.T) {
	assignments := map[string][]string{"Backend": {"bob", "alice"}, "Frontend": {"alice", "carol"}}

	assert.Equal(t, []string{"alice", "bob", "carol"}, Reviewers(assignments))
}
```

- [ ] **Step 6: Run all pipeline tests to verify green**

Run: `go test ./internal/pipeline/ -v`
Expected: PASS for all new `TestDiffAndSelect_*`, `TestReviewers_*`, and all pre-existing tests (unchanged).

- [ ] **Step 7: Commit**

```bash
git add internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go
git commit -m "feat: add per-scope DiffAndSelect core"
```

---

## Task 2: Migrate the `check` subcommand onto DiffAndSelect

**Files:**
- Modify: `cmd/daylight/main.go:108` (the `runCheck` body)

- [ ] **Step 1: Replace the ResolveAndSelect call in `runCheck`**

In `cmd/daylight/main.go`, replace this line:

```go
	reviewers := pipeline.ResolveAndSelect(changedFiles, sections, *author, selection.RandomStrategy{}, printf)
```

with:

```go
	assignments := pipeline.DiffAndSelect(changedFiles, sections, *author, nil, selection.RandomStrategy{}, printf)
	reviewers := pipeline.Reviewers(assignments)
```

(A local dry-run has no prior note, so `previous` is `nil` — every matched scope is treated as new. The rest of `runCheck` is unchanged.)

- [ ] **Step 2: Build to verify it compiles**

Run: `go build ./...`
Expected: success. (`ResolveAndSelect` still exists and is still used by `Run`, so the package compiles.)

- [ ] **Step 3: Manually verify the check subcommand against the fixture**

Run: `go run ./cmd/daylight check -codeowners CODEOWNERS_test $(grep -oE '^/[^ ]+' CODEOWNERS_test | head -1 | sed 's#^/##')`
Expected: prints `=== FILE MATCHING ===`, `=== SCOPE DIFF ===` with `new — pool: …`, and a `=== RESULT ===` line listing reviewers (or "no reviewers would be assigned"). No panic, no API calls.

- [ ] **Step 4: Commit**

```bash
git add cmd/daylight/main.go
git commit -m "refactor: check subcommand uses DiffAndSelect"
```

---

## Task 3: JSON note state + rewire Run (atomic)

This task changes `FormatNote`/`ParseNote` signatures, which forces `Run`, `idempotency_test.go`, and the Run tests in `pipeline_test.go` to change together, and makes `ResolveAndSelect`/`filesHash` unused (so they're removed). One commit.

**Files:**
- Modify: `internal/pipeline/idempotency.go` (JSON encode/parse)
- Modify: `internal/pipeline/idempotency_test.go` (round-trip tests)
- Modify: `internal/pipeline/pipeline.go` (rewire `Run`, remove `ResolveAndSelect` + `filesHash`)
- Modify: `internal/pipeline/pipeline_test.go` (Run tests)

- [ ] **Step 1: Rewrite the note round-trip tests**

Replace the whole body of `internal/pipeline/idempotency_test.go` (keep the ABOUTME header) with:

```go
// ABOUTME: tests for idempotency note formatting and parsing
// ABOUTME: notes are posted as confidential GitLab MR comments to track processed runs
package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatAndParseNote_RoundTrip(t *testing.T) {
	assignments := map[string][]string{"Backend Team": {"alice", "bob"}, "Frontend": {"carol"}}

	note := FormatNote("abc123", assignments)
	sha, got, ok := ParseNote(note)

	assert.True(t, ok)
	assert.Equal(t, "abc123", sha)
	assert.Equal(t, assignments, got)
}

func TestFormatAndParseNote_EmptyAssignments(t *testing.T) {
	note := FormatNote("abc123", map[string][]string{})
	sha, got, ok := ParseNote(note)

	assert.True(t, ok)
	assert.Equal(t, "abc123", sha)
	assert.Empty(t, got)
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

- [ ] **Step 2: Run the note tests to verify they fail**

Run: `go test ./internal/pipeline/ -run TestFormatAndParseNote -v`
Expected: compile error — `FormatNote`/`ParseNote` signature mismatch. (The whole package won't compile until Steps 3–5 land; that's expected for this atomic task.)

- [ ] **Step 3: Rewrite `idempotency.go` to JSON state**

Replace the whole body of `internal/pipeline/idempotency.go` with:

```go
// ABOUTME: formats and parses the internal MR note that records processed runs
// ABOUTME: note body is "daylight:processed " followed by JSON {sha, assignments}
package pipeline

import (
	"encoding/json"
	"strings"
)

const notePrefix = "daylight:processed "

type noteState struct {
	SHA         string              `json:"sha"`
	Assignments map[string][]string `json:"assignments"`
}

// FormatNote encodes the processed commit SHA and the scope→reviewers map as a note body.
func FormatNote(sha string, assignments map[string][]string) string {
	data, _ := json.Marshal(noteState{SHA: sha, Assignments: assignments})
	return notePrefix + string(data)
}

// ParseNote decodes a note body produced by FormatNote. ok is false for any body that is
// not a daylight note or whose SHA is missing.
func ParseNote(body string) (sha string, assignments map[string][]string, ok bool) {
	if !strings.HasPrefix(body, notePrefix) {
		return "", nil, false
	}
	var state noteState
	if err := json.Unmarshal([]byte(body[len(notePrefix):]), &state); err != nil {
		return "", nil, false
	}
	if state.SHA == "" {
		return "", nil, false
	}
	return state.SHA, state.Assignments, true
}
```

- [ ] **Step 4: Rewire `Run` and remove the dead functions in `pipeline.go`**

In `internal/pipeline/pipeline.go`:

(a) Remove the now-unused imports `"crypto/sha256"`, `"encoding/hex"` from the import block. Keep `"sort"` (used by `Reviewers`) and `"strings"`.

(b) Replace the entire `Run` function (lines 20–77) with:

```go
func Run(ctx context.Context, cfg config.Config, gl gitlab.GitLabClient, strategy selection.SelectionStrategy) error {
	if cfg.IsDraft {
		log.Println("MR is a draft, skipping")
		return nil
	}

	notes, err := gl.MRNotes(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR notes: %w", err)
	}

	var prevSHA string
	var prevAssignments map[string][]string
	for _, note := range notes {
		if sha, assignments, ok := ParseNote(note.Body); ok {
			prevSHA, prevAssignments = sha, assignments
			break
		}
	}
	if prevSHA == cfg.CommitSHA {
		log.Println("already processed this commit, skipping")
		return nil
	}

	files, author, err := gl.MRChanges(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR changes: %w", err)
	}
	log.Printf("author: %s, changed files: %d", author, len(files))

	content, err := gl.CODEOWNERSContent(ctx, cfg.ProjectID, cfg.CommitSHA)
	if err != nil {
		return fmt.Errorf("fetch CODEOWNERS: %w", err)
	}
	sections := ownership.Parse(content)

	assignments := DiffAndSelect(files, sections, author, prevAssignments, strategy, log.Printf)
	newReviewers := Reviewers(assignments)
	prevReviewers := Reviewers(prevAssignments)

	if len(newReviewers) == 0 && len(prevReviewers) == 0 {
		log.Println("no reviewers to assign")
		return nil
	}

	log.Printf("=== RESULT ===")
	if equalStrings(newReviewers, prevReviewers) {
		log.Printf("reviewers unchanged: %s", strings.Join(newReviewers, ", "))
	} else {
		log.Printf("assigning reviewers: %s", strings.Join(newReviewers, ", "))
		if err := gl.SetReviewers(ctx, cfg.ProjectID, cfg.MRIID, newReviewers); err != nil {
			return fmt.Errorf("set reviewers: %w", err)
		}
	}

	if err := gl.PostInternalNote(ctx, cfg.ProjectID, cfg.MRIID, FormatNote(cfg.CommitSHA, assignments)); err != nil {
		return fmt.Errorf("post note: %w", err)
	}

	log.Println("done")
	return nil
}

// equalStrings reports whether two sorted string slices are element-wise equal.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

(c) Delete the entire `ResolveAndSelect` function (the old one, roughly old lines 79–155) and the `filesHash` function (old lines 157–163). `DiffAndSelect`, `selectN`, and `Reviewers` from Task 1 stay.

- [ ] **Step 5: Update the Run tests in `pipeline_test.go`**

In `internal/pipeline/pipeline_test.go`:

(a) Replace `TestRun_AlreadyProcessedBySHA_ExitsEarly` with (the `FormatNote` call now takes a map):

```go
func TestRun_AlreadyProcessedBySHA_ExitsEarly(t *testing.T) {
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{{Body: FormatNote("sha1", map[string][]string{"Backend": {"alice"}})}}, nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, nil)

	require.NoError(t, err)
}
```

(b) Delete `TestRun_AlreadyProcessedByFilesHash_ExitsEarly` entirely. (It guards the `files_hash` skip, which the design intentionally removes — the new behavior is covered by `TestRun_ScopesUnchanged_DoesNotReassign` below.)

(c) Update the note assertion in `TestRun_HappyPath_AssignsReviewerAndPostsNote`. Replace:

```go
	assert.Contains(t, postedNote, "daylight:processed sha=sha1")
```

with:

```go
	sha, postedAssignments, ok := ParseNote(postedNote)
	assert.True(t, ok)
	assert.Equal(t, "sha1", sha)
	assert.Len(t, postedAssignments["Backend"], 1)
```

(d) Append two new Run tests:

```go
func TestRun_ScopesUnchanged_DoesNotReassign(t *testing.T) {
	setCalled := false
	var postedNote string
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{{Body: FormatNote("oldsha", map[string][]string{"Backend": {"alice"}})}}, nil
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
		postInternalNote: func(_ context.Context, _, _, b string) error {
			postedNote = b
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.False(t, setCalled, "reviewers unchanged, SetReviewers must not be called")
	sha, postedAssignments, ok := ParseNote(postedNote)
	assert.True(t, ok)
	assert.Equal(t, "sha1", sha)
	assert.Equal(t, []string{"alice"}, postedAssignments["Backend"])
}

func TestRun_AllScopesRemoved_ClearsReviewers(t *testing.T) {
	clearCalled := false
	var assigned []string
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{{Body: FormatNote("oldsha", map[string][]string{"Backend": {"alice"}})}}, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/docs/readme.md"}, "author", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "[Backend][1]\n/src/ @alice\n", nil
		},
		setReviewers: func(_ context.Context, _, _ string, u []string) error {
			clearCalled = true
			assigned = u
			return nil
		},
		postInternalNote: func(_ context.Context, _, _, _ string) error { return nil },
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.True(t, clearCalled, "reviewers must be cleared when all scopes vanish")
	assert.Empty(t, assigned)
}
```

- [ ] **Step 6: Run the full pipeline test suite to verify green**

Run: `go test ./internal/pipeline/ -v`
Expected: PASS. Confirm specifically that `TestRun_ScopesUnchanged_DoesNotReassign`, `TestRun_AllScopesRemoved_ClearsReviewers`, the updated `TestRun_HappyPath_AssignsReviewerAndPostsNote`, `TestRun_MultiReviewer_AssignsN`, `TestRun_AuthorExcludedFromSelection`, and `TestRun_NoOwners_Noop` all pass.

- [ ] **Step 7: Build the whole module and run all tests**

Run: `go build ./... && go test ./...`
Expected: build succeeds (no unused `ResolveAndSelect`/`filesHash`), all packages green.

- [ ] **Step 8: Commit**

```bash
git add internal/pipeline/idempotency.go internal/pipeline/idempotency_test.go internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go
git commit -m "feat: per-scope reviewer stability via scope-diffed note state"
```

---

## Final verification

- [ ] **Step 1: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: clean, all green.

- [ ] **Step 2: End-to-end dry run of the check subcommand**

Run: `go run ./cmd/daylight check -codeowners CODEOWNERS_test -author someuser $(grep -oE '^/[^ ]+' CODEOWNERS_test | head -2 | sed 's#^/##' | tr '\n' ' ')`
Expected: file-matching lines, scope-diff lines (all `new`), and a result line. No panic.

---

## Spec coverage check

- Scope present in both → kept verbatim: Task 1 `TestDiffAndSelect_PresentScope_KeptVerbatim`, Task 3 `TestRun_ScopesUnchanged_DoesNotReassign`.
- New scope → reviewer selected: Task 1 `TestDiffAndSelect_NewScopeAdded_OnlyNewSelected`.
- Removed scope → reviewer dropped: Task 1 `TestDiffAndSelect_ScopeRemoved_Dropped`.
- All scopes removed → empty-clear: Task 1 `TestDiffAndSelect_AllScopesRemoved_Empty`, Task 3 `TestRun_AllScopesRemoved_ClearsReviewers`.
- Cross-scope dedup + author exclusion preserved: Task 1 `TestDiffAndSelect_NewScope_ExcludesAlreadyAssigned`, `TestDiffAndSelect_NewScope_ExcludesAuthor`.
- Exhausted pool tolerated: Task 1 `TestDiffAndSelect_NewScope_PoolExhausted_NoReviewer`.
- JSON note state (sha + assignments), spaces in scope names, empty assignments: Task 3 idempotency tests.
- `sha` idempotency skip retained: Task 3 `TestRun_AlreadyProcessedBySHA_ExitsEarly`.
- Conditional `SetReviewers` (only when union changes): Task 3 `TestRun_ScopesUnchanged_DoesNotReassign`.
- `files_hash` dropped: Task 3 removes `filesHash` and the corresponding test.
- `check` subcommand still works: Task 2 manual verification.
