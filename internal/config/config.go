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
