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
