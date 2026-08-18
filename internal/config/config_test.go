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
	orig := make(map[string]*string, len(vars))
	for k, v := range vars {
		if prev, ok := os.LookupEnv(k); ok {
			prev := prev
			orig[k] = &prev
		} else {
			orig[k] = nil
		}
		os.Setenv(k, v)
	}
	return func() {
		for k, v := range orig {
			if v == nil {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, *v)
			}
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
	required := []string{
		"CI_PROJECT_ID",
		"CI_MERGE_REQUEST_IID",
		"CI_COMMIT_SHA",
		"DAYLIGHT_GITLAB_TOKEN",
	}
	for _, missing := range required {
		t.Run("missing_"+missing, func(t *testing.T) {
			vars := requiredVars()
			delete(vars, missing)
			defer withEnv(vars)()
			os.Unsetenv(missing)

			_, err := Load()

			require.Error(t, err)
		})
	}
}

func TestLoad_IsDraftTrue(t *testing.T) {
	vars := requiredVars()
	vars["CI_MERGE_REQUEST_DRAFT"] = "true"
	defer withEnv(vars)()

	cfg, err := Load()

	require.NoError(t, err)
	assert.True(t, cfg.IsDraft)
}

func TestLoad_IsDraftFalse(t *testing.T) {
	vars := requiredVars()
	vars["CI_MERGE_REQUEST_DRAFT"] = "false"
	defer withEnv(vars)()

	cfg, err := Load()

	require.NoError(t, err)
	assert.False(t, cfg.IsDraft)
}

func TestLoad_URLFallbackToGitLabCom(t *testing.T) {
	defer withEnv(requiredVars())()
	os.Unsetenv("CI_SERVER_URL")
	os.Unsetenv("DAYLIGHT_GITLAB_URL")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com", cfg.GitLabURL)
}

func TestLoad_ReuseSharedReviewersTrue(t *testing.T) {
	vars := requiredVars()
	vars["DAYLIGHT_REUSE_SHARED_REVIEWERS"] = "true"
	defer withEnv(vars)()

	cfg, err := Load()

	require.NoError(t, err)
	assert.True(t, cfg.ReuseSharedReviewers)
}

func TestLoad_ReuseSharedReviewersDefaultsFalse(t *testing.T) {
	defer withEnv(requiredVars())()
	os.Unsetenv("DAYLIGHT_REUSE_SHARED_REVIEWERS")

	cfg, err := Load()

	require.NoError(t, err)
	assert.False(t, cfg.ReuseSharedReviewers)
}

func TestLoad_ReuseSharedReviewersNonTrueIsFalse(t *testing.T) {
	vars := requiredVars()
	vars["DAYLIGHT_REUSE_SHARED_REVIEWERS"] = "1"
	defer withEnv(vars)()

	cfg, err := Load()

	require.NoError(t, err)
	assert.False(t, cfg.ReuseSharedReviewers)
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
