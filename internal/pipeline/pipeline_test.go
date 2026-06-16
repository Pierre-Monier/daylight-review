// ABOUTME: tests for the assignment pipeline using a hand-rolled mock GitLab client
// ABOUTME: one test per early-exit condition plus a happy path and author-exclusion test
package pipeline

import (
	"context"
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
		postInternalNote: func(_ context.Context, _, _, _ string) error { return nil },
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
		postInternalNote: func(_ context.Context, _, _, _ string) error { return nil },
	}

	err := Run(context.Background(), baseCfg(), cl, selection.RandomStrategy{})

	require.NoError(t, err)
	assert.Equal(t, []string{"bob"}, assignedReviewers)
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

	got := DiffAndSelect(files, sections, "", previous, firstStrategy{}, noopPrintf)

	assert.Equal(t, []string{"alice", "bob"}, got["Backend"])
}

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
