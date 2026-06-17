# Feedback Note Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On the first time Daylight processes a merge request, post one short, confidential MR note inviting the team to send feedback via a hardcoded form link.

**Architecture:** A dedicated `internal/pipeline/feedback.go` owns the form URL, note wording, and visibility. The GitLab client's note-posting method is generalized to take an explicit `confidential` flag (the seam that makes visibility easy to change later). `pipeline.Run` posts the feedback note once — only on the first run against an MR (`prevSHA == ""`), best-effort so it never fails the job.

**Tech Stack:** Go, `testify` (`assert`/`require`), hand-rolled mock GitLab client in `pipeline_test.go`.

---

## File Structure

- `internal/gitlab/client.go` — **Modify.** Replace `PostInternalNote` with `PostNote(..., confidential bool)` on both the `GitLabClient` interface and `Client`.
- `internal/pipeline/pipeline.go` — **Modify.** Update the processed-note call to `PostNote(..., true)`; post the feedback note after it on first runs.
- `internal/pipeline/feedback.go` — **Create.** Owns `feedbackURL`, note body, and visibility.
- `internal/pipeline/feedback_test.go` — **Create.** Tests the feedback note body and visibility.
- `internal/pipeline/pipeline_test.go` — **Modify.** Update the mock to the new `PostNote` signature; refactor `TestRun_HappyPath` to record multiple notes; add feedback-note tests.

---

## Task 1: Generalize note posting to `PostNote`

Pure refactor — no behavior change. Renames `PostInternalNote` to `PostNote` with an explicit `confidential` flag and updates all callers and the test mock. Because feedback is not wired yet, every run still posts exactly one note, so all existing tests stay green.

**Files:**
- Modify: `internal/gitlab/client.go:26` (interface) and `internal/gitlab/client.go:129-134` (impl)
- Modify: `internal/pipeline/pipeline.go:74`
- Modify: `internal/pipeline/pipeline_test.go:22`, `:37-39`, and every `postInternalNote:` call site

- [ ] **Step 1: Update the interface method**

In `internal/gitlab/client.go`, replace line 26:

```go
	PostNote(ctx context.Context, projectID, mrIID, body string, confidential bool) error
```

- [ ] **Step 2: Update the `Client` implementation**

In `internal/gitlab/client.go`, replace the `PostInternalNote` method (lines 129-134) with:

```go
func (c *Client) PostNote(ctx context.Context, projectID, mrIID, body string, confidential bool) error {
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s/notes", c.baseURL, projectID, mrIID)
	return c.doJSON(ctx, http.MethodPost, url,
		map[string]any{"body": body, "confidential": confidential},
		http.StatusCreated)
}
```

- [ ] **Step 3: Update the pipeline caller**

In `internal/pipeline/pipeline.go`, replace line 74:

```go
	if err := gl.PostNote(ctx, cfg.ProjectID, cfg.MRIID, FormatNote(cfg.CommitSHA, assignments), true); err != nil {
```

- [ ] **Step 4: Update the mock field and method**

In `internal/pipeline/pipeline_test.go`, replace the `postInternalNote` field (line 22):

```go
	postNote func(context.Context, string, string, string, bool) error
```

and replace the method (lines 37-39):

```go
func (m *mockClient) PostNote(ctx context.Context, p, mr, b string, c bool) error {
	return m.postNote(ctx, p, mr, b, c)
}
```

- [ ] **Step 5: Update every mock call site**

In `internal/pipeline/pipeline_test.go`, rename each `postInternalNote:` key to `postNote:` and add a `, _ bool` parameter. There are six call sites. The two distinct forms become:

```go
		postNote: func(_ context.Context, _, _, b string, _ bool) error {
			postedNote = b
			return nil
		},
```

and

```go
		postNote: func(_ context.Context, _, _, _ string, _ bool) error { return nil },
```

- [ ] **Step 6: Run the full suite to verify it still passes**

Run: `go test ./...`
Expected: PASS (pure refactor, behavior unchanged).

- [ ] **Step 7: Commit**

```bash
git add internal/gitlab/client.go internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go
git commit -m "refactor: PostNote takes explicit confidential flag"
```

---

## Task 2: Add the feedback note builder

**Files:**
- Create: `internal/pipeline/feedback.go`
- Test: `internal/pipeline/feedback_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/pipeline/feedback_test.go`:

```go
// ABOUTME: tests for the one-time feedback note body and visibility
package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeedbackNote_ContainsURLAndIsConfidential(t *testing.T) {
	body, confidential := feedbackNote()

	assert.Contains(t, body, feedbackURL)
	assert.True(t, confidential)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pipeline/ -run TestFeedbackNote -v`
Expected: build failure — `undefined: feedbackNote` and `undefined: feedbackURL`.

- [ ] **Step 3: Write the implementation**

Create `internal/pipeline/feedback.go`:

```go
// ABOUTME: builds the one-time feedback note inviting MR teams to send feedback
// ABOUTME: owns the feedback form URL, the note wording, and the note's visibility
package pipeline

import "fmt"

// feedbackURL is the destination of the feedback link in the MR note.
const feedbackURL = "https://PLACEHOLDER-FORM-URL"

// feedbackNote returns the body and visibility of the one-time feedback note
// posted on the first run against a merge request.
func feedbackNote() (body string, confidential bool) {
	body = fmt.Sprintf("🌅 Reviewers on this MR were assigned by **Daylight**. "+
		"Hit a bug or have feedback? [Let us know](%s).", feedbackURL)
	return body, true
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/pipeline/ -run TestFeedbackNote -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/feedback.go internal/pipeline/feedback_test.go
git commit -m "feat: add feedback note builder"
```

---

## Task 3: Post the feedback note once per MR

Wire the feedback note into `pipeline.Run`: after the processed note, on first runs only (`prevSHA == ""`), best-effort. Wiring this makes the first run post two notes, which breaks `TestRun_HappyPath`'s single-variable note capture — so this task also refactors that test to record all posted notes.

**Files:**
- Modify: `internal/pipeline/pipeline.go` (after line 76)
- Modify: `internal/pipeline/pipeline_test.go` (imports, `TestRun_HappyPath`, new tests, helper)

- [ ] **Step 1: Add the note-recording helper and type to the test file**

In `internal/pipeline/pipeline_test.go`, add the following just above `firstStrategy` (near line 235):

```go
type recordedNote struct {
	body         string
	confidential bool
}

func findProcessedNote(notes []recordedNote) (string, bool) {
	for _, n := range notes {
		if _, _, ok := ParseNote(n.body); ok {
			return n.body, true
		}
	}
	return "", false
}
```

- [ ] **Step 2: Write the failing feedback tests**

In `internal/pipeline/pipeline_test.go`, add these three tests. Add `"fmt"` to the import block (used by the error test):

```go
func TestRun_FirstRun_PostsFeedbackNote(t *testing.T) {
	var posted []recordedNote
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
		setReviewers: func(_ context.Context, _, _ string, _ []string) error { return nil },
		postNote: func(_ context.Context, _, _, b string, c bool) error {
			posted = append(posted, recordedNote{b, c})
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, selection.RandomStrategy{})

	require.NoError(t, err)
	wantBody, wantConf := feedbackNote()
	assert.Contains(t, posted, recordedNote{wantBody, wantConf})
}

func TestRun_NotFirstRun_NoFeedbackNote(t *testing.T) {
	var posted []recordedNote
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
		setReviewers: func(_ context.Context, _, _ string, _ []string) error { return nil },
		postNote: func(_ context.Context, _, _, b string, c bool) error {
			posted = append(posted, recordedNote{b, c})
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	wantBody, _ := feedbackNote()
	for _, n := range posted {
		assert.NotEqual(t, wantBody, n.body, "feedback note must not post after the first run")
	}
}

func TestRun_FeedbackNoteError_RunStillSucceeds(t *testing.T) {
	wantBody, _ := feedbackNote()
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
		setReviewers: func(_ context.Context, _, _ string, _ []string) error { return nil },
		postNote: func(_ context.Context, _, _, b string, _ bool) error {
			if b == wantBody {
				return fmt.Errorf("boom")
			}
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, selection.RandomStrategy{})

	require.NoError(t, err)
}
```

- [ ] **Step 3: Run the new tests to verify they fail**

Run: `go test ./internal/pipeline/ -run 'TestRun_FirstRun_PostsFeedbackNote|TestRun_NotFirstRun_NoFeedbackNote|TestRun_FeedbackNoteError' -v`
Expected: `TestRun_FirstRun_PostsFeedbackNote` FAILS (feedback note never posted). The other two PASS already (no feedback wired yet), which is fine — they lock in behavior once wiring lands.

- [ ] **Step 4: Wire the feedback note into the pipeline**

In `internal/pipeline/pipeline.go`, insert the following immediately after the processed-note block (after line 76, before `log.Println("done")`):

```go
	if prevSHA == "" {
		body, confidential := feedbackNote()
		if err := gl.PostNote(ctx, cfg.ProjectID, cfg.MRIID, body, confidential); err != nil {
			log.Printf("warning: post feedback note: %v", err)
		}
	}
```

- [ ] **Step 5: Run the new tests to verify they pass**

Run: `go test ./internal/pipeline/ -run 'TestRun_FirstRun_PostsFeedbackNote|TestRun_NotFirstRun_NoFeedbackNote|TestRun_FeedbackNoteError' -v`
Expected: all three PASS.

- [ ] **Step 6: Fix `TestRun_HappyPath` to record all posted notes**

Wiring now makes the happy-path run post two notes, so its single-variable capture must become slice-based. In `internal/pipeline/pipeline_test.go`, replace the body of `TestRun_HappyPath_AssignsReviewerAndPostsNote` with:

```go
func TestRun_HappyPath_AssignsReviewerAndPostsNote(t *testing.T) {
	var assignedReviewers []string
	var posted []recordedNote
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
		postNote: func(_ context.Context, _, _, b string, c bool) error {
			posted = append(posted, recordedNote{b, c})
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, selection.RandomStrategy{})

	require.NoError(t, err)
	assert.Len(t, assignedReviewers, 1)
	assert.Contains(t, []string{"alice", "bob"}, assignedReviewers[0])
	body, ok := findProcessedNote(posted)
	require.True(t, ok, "a processed note must be posted")
	sha, postedAssignments, ok := ParseNote(body)
	assert.True(t, ok)
	assert.Equal(t, "sha1", sha)
	assert.Len(t, postedAssignments["Backend"], 1)
}
```

- [ ] **Step 7: Run the full suite to verify everything passes**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/pipeline/pipeline.go internal/pipeline/pipeline_test.go
git commit -m "feat: post one-time feedback note on first MR run"
```
