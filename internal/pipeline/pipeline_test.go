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
