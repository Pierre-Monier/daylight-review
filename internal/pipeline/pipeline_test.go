// ABOUTME: tests for the assignment pipeline using a hand-rolled mock GitLab client
// ABOUTME: one test per early-exit condition plus a happy path and author-exclusion test
package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/ownership"
	"github.com/daylight-review/daylight/internal/selection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	mrNotes           func(context.Context, string, string) ([]gitlab.Note, error)
	mrChanges         func(context.Context, string, string) ([]string, string, error)
	codeownersContent func(context.Context, string, string) (string, error)
	setReviewers      func(context.Context, string, string, []string) error
	postNote          func(context.Context, string, string, string, bool) error
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
func (m *mockClient) PostNote(ctx context.Context, p, mr, b string, c bool) error {
	return m.postNote(ctx, p, mr, b, c)
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
			return []gitlab.Note{{Body: FormatNote("sha1", map[string][]string{"Backend": {"alice"}}, nil)}}, nil
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
	sha, postedAssignments, _, ok := ParseNote(body)
	assert.True(t, ok)
	assert.Equal(t, "sha1", sha)
	assert.Len(t, postedAssignments["Backend"], 1)
}

func TestRun_MultiReviewer_AssignsN(t *testing.T) {
	var assignedReviewers []string
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return nil, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/src/main.go"}, "dave", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "[Backend][2]\n/src/ @alice @bob @carol\n", nil
		},
		setReviewers: func(_ context.Context, _, _ string, u []string) error {
			assignedReviewers = u
			return nil
		},
		postNote: func(_ context.Context, _, _, _ string, _ bool) error { return nil },
	}

	err := Run(context.Background(), baseCfg(), cl, selection.RandomStrategy{})

	require.NoError(t, err)
	assert.Len(t, assignedReviewers, 2)
	assert.Equal(t, 2, len(assignedReviewers))
	// both must be distinct and from the candidate pool
	assert.NotEqual(t, assignedReviewers[0], assignedReviewers[1])
	for _, r := range assignedReviewers {
		assert.Contains(t, []string{"alice", "bob", "carol"}, r)
	}
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
		postNote: func(_ context.Context, _, _, _ string, _ bool) error { return nil },
	}

	err := Run(context.Background(), baseCfg(), cl, selection.RandomStrategy{})

	require.NoError(t, err)
	assert.Equal(t, []string{"bob"}, assignedReviewers)
}

func TestRun_ScopesUnchanged_DoesNotReassign(t *testing.T) {
	setCalled := false
	var postedNote string
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{{Body: FormatNote("oldsha", map[string][]string{"Backend": {"alice"}}, nil)}}, nil
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
		postNote: func(_ context.Context, _, _, b string, _ bool) error {
			postedNote = b
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.False(t, setCalled, "reviewers unchanged, SetReviewers must not be called")
	sha, postedAssignments, _, ok := ParseNote(postedNote)
	assert.True(t, ok)
	assert.Equal(t, "sha1", sha)
	assert.Equal(t, []string{"alice"}, postedAssignments["Backend"])
}

func TestRun_AllScopesRemoved_ClearsReviewers(t *testing.T) {
	clearCalled := false
	var assigned []string
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{{Body: FormatNote("oldsha", map[string][]string{"Backend": {"alice"}}, nil)}}, nil
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
		postNote: func(_ context.Context, _, _, _ string, _ bool) error { return nil },
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.True(t, clearCalled, "reviewers must be cleared when all scopes vanish")
	assert.Empty(t, assigned)
}

type recordedNote struct {
	body         string
	confidential bool
}

func findProcessedNote(notes []recordedNote) (string, bool) {
	for _, n := range notes {
		if _, _, _, ok := ParseNote(n.body); ok {
			return n.body, true
		}
	}
	return "", false
}

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

	got, _ := DiffAndSelect(files, sections, "", previous, nil, false, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"alice", "bob"}, got["Backend"])
}

func TestDiffAndSelect_NewScopeAdded_OnlyNewSelected(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n[Frontend][1]\n/web/ @carol\n")
	files := []string{"/src/main.go", "/web/app.js"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, _ := DiffAndSelect(files, sections, "", previous, nil, false,firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"alice"}, got["Backend"])
	assert.Equal(t, []string{"carol"}, got["Frontend"])
}

func TestDiffAndSelect_ScopeRemoved_Dropped(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n[Frontend][1]\n/web/ @carol\n")
	files := []string{"/src/main.go"}
	previous := map[string][]string{"Backend": {"alice"}, "Frontend": {"carol"}}

	got, _ := DiffAndSelect(files, sections, "", previous, nil, false, firstStrategy{}, noopPrintf)

	assert.Equal(t, map[string][]string{"Backend": {"alice"}}, got)
}

func TestDiffAndSelect_AllScopesRemoved_Empty(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n")
	files := []string{"/docs/readme.md"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, _ := DiffAndSelect(files, sections, "", previous, nil, false, firstStrategy{}, noopPrintf)

	assert.Empty(t, got)
}

func TestDiffAndSelect_NewScope_ExcludesAlreadyAssigned(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n[Frontend][1]\n/web/ @alice @dave\n")
	files := []string{"/src/main.go", "/web/app.js"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, _ := DiffAndSelect(files, sections, "", previous, nil, false, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"alice"}, got["Backend"])
	assert.Equal(t, []string{"dave"}, got["Frontend"])
}

func TestDiffAndSelect_NewScope_ExcludesAuthor(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice @bob\n")
	files := []string{"/src/main.go"}

	got, _ := DiffAndSelect(files, sections, "alice", nil, nil, false, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"bob"}, got["Backend"])
}

func TestDiffAndSelect_NewScope_PoolExhausted_NoReviewer(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n")
	files := []string{"/src/main.go"}

	got, _ := DiffAndSelect(files, sections, "alice", nil, nil, false, firstStrategy{}, noopPrintf)

	_, has := got["Backend"]
	assert.False(t, has)
}

func TestDiffAndSelect_OOOReviewer_Replaced(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice @bob\n")
	files := []string{"/src/main.go"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, unfilled := DiffAndSelect(files, sections, "", previous, []string{"alice"}, true, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"bob"}, got["Backend"])
	assert.Empty(t, unfilled)
}

func TestDiffAndSelect_OOOReviewer_StickyAcrossScopes(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice @bob\n[Frontend][1]\n/web/ @alice @dave\n")
	files := []string{"/src/main.go", "/web/app.js"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, unfilled := DiffAndSelect(files, sections, "", previous, []string{"alice"}, true, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"bob"}, got["Backend"])
	assert.Equal(t, []string{"dave"}, got["Frontend"])
	assert.Empty(t, unfilled)
}

func TestDiffAndSelect_OOOBackfill_DoesNotDoubleBookOtherScopeOwner(t *testing.T) {
	sections := ownership.Parse("[A][1]\n/a/ @alice @bob @carol\n[B][1]\n/b/ @bob\n")
	files := []string{"/a/main.go", "/b/main.go"}
	previous := map[string][]string{"A": {"alice"}, "B": {"bob"}}

	// Scope map iteration order is randomized by the Go runtime, so run enough times to
	// surface the bug regardless of which scope DiffAndSelect happens to visit first.
	for i := 0; i < 30; i++ {
		got, unfilled := DiffAndSelect(files, sections, "", previous, []string{"alice"}, true, firstStrategy{}, noopPrintf)

		assert.Equal(t, []string{"carol"}, got["A"], "A's replacement must not steal B's kept owner")
		assert.Equal(t, []string{"bob"}, got["B"], "B must keep its stored reviewer")
		assert.Empty(t, unfilled)
	}
}

func TestDiffAndSelect_TwoOOOInScope_ReplacesEachOrUnfilled(t *testing.T) {
	sections := ownership.Parse("[Backend][2]\n/src/ @alice @bob @carol\n")
	files := []string{"/src/main.go"}
	previous := map[string][]string{"Backend": {"alice", "bob"}}

	got, unfilled := DiffAndSelect(files, sections, "", previous, []string{"alice", "bob"}, true, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"carol"}, got["Backend"])
	assert.Equal(t, []Unfilled{{Scope: "Backend", Username: "bob"}}, unfilled)
}

func TestDiffAndSelect_OOOReviewer_NoReplacement_Unfilled(t *testing.T) {
	sections := ownership.Parse("[Backend][1]\n/src/ @alice\n")
	files := []string{"/src/main.go"}
	previous := map[string][]string{"Backend": {"alice"}}

	got, unfilled := DiffAndSelect(files, sections, "", previous, []string{"alice"}, true, firstStrategy{}, noopPrintf)

	assert.NotContains(t, got, "Backend")
	assert.Equal(t, []Unfilled{{Scope: "Backend", Username: "alice"}}, unfilled)
}

func TestReviewers_SortedDeduped(t *testing.T) {
	assignments := map[string][]string{"Backend": {"bob", "alice"}, "Frontend": {"alice", "carol"}}

	assert.Equal(t, []string{"alice", "bob", "carol"}, Reviewers(assignments))
}

func TestRun_NewScopeAdded_KeepsExistingAddsNew(t *testing.T) {
	var assigned []string
	var postedNote string
	cl := &mockClient{
		mrNotes: func(_ context.Context, _, _ string) ([]gitlab.Note, error) {
			return []gitlab.Note{{Body: FormatNote("oldsha", map[string][]string{"Backend": {"alice"}}, nil)}}, nil
		},
		mrChanges: func(_ context.Context, _, _ string) ([]string, string, error) {
			return []string{"/src/main.go", "/web/app.js"}, "author", nil
		},
		codeownersContent: func(_ context.Context, _, _ string) (string, error) {
			return "[Backend][1]\n/src/ @alice\n[Frontend][1]\n/web/ @carol\n", nil
		},
		setReviewers: func(_ context.Context, _, _ string, u []string) error {
			assigned = u
			return nil
		},
		postNote: func(_ context.Context, _, _, b string, _ bool) error {
			postedNote = b
			return nil
		},
	}

	err := Run(context.Background(), baseCfg(), cl, firstStrategy{})

	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "carol"}, assigned)
	sha, postedAssignments, _, ok := ParseNote(postedNote)
	assert.True(t, ok)
	assert.Equal(t, "sha1", sha)
	assert.Equal(t, []string{"alice"}, postedAssignments["Backend"])
	assert.Equal(t, []string{"carol"}, postedAssignments["Frontend"])
}

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
			return []gitlab.Note{{Body: FormatNote("oldsha", map[string][]string{"Backend": {"alice"}}, nil)}}, nil
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
