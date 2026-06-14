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
