// ABOUTME: entry point for the daylight CLI — wires config, GitLab client, and pipeline
// ABOUTME: exits with code 1 and prints the error on any failure
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/pipeline"
	"github.com/daylight-review/daylight/internal/selection"
)

func main() {
	fs := flag.NewFlagSet("daylight", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Assigns reviewers to a GitLab MR based on CODEOWNERS.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Configuration via environment variables (set automatically in GitLab CI):")
		fmt.Fprintln(os.Stderr, "  DAYLIGHT_GITLAB_TOKEN   GitLab API token with api scope (required)")
		fmt.Fprintln(os.Stderr, "  CI_PROJECT_ID           GitLab project ID (required)")
		fmt.Fprintln(os.Stderr, "  CI_MERGE_REQUEST_IID    Merge request IID (required)")
		fmt.Fprintln(os.Stderr, "  CI_COMMIT_SHA           Commit SHA (required)")
		fmt.Fprintln(os.Stderr, "  DAYLIGHT_GITLAB_URL     GitLab instance URL (optional, defaults to gitlab.com)")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags override the corresponding env var (useful for local testing):")
		fmt.Fprintln(os.Stderr, "  -token, -project-id, -mr-iid, -sha, -url")
	}
	token := fs.String("token", "", "")
	projectID := fs.String("project-id", "", "")
	mrIID := fs.String("mr-iid", "", "")
	sha := fs.String("sha", "", "")
	url := fs.String("url", "", "")
	_ = fs.Parse(os.Args[1:])

	setIfProvided := func(envKey, val string) {
		if val != "" {
			os.Setenv(envKey, val)
		}
	}
	setIfProvided("DAYLIGHT_GITLAB_TOKEN", *token)
	setIfProvided("CI_PROJECT_ID", *projectID)
	setIfProvided("CI_MERGE_REQUEST_IID", *mrIID)
	setIfProvided("CI_COMMIT_SHA", *sha)
	setIfProvided("DAYLIGHT_GITLAB_URL", *url)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client := gitlab.New(cfg.GitLabURL, cfg.Token)

	if err := pipeline.Run(context.Background(), cfg, client, selection.RandomStrategy{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
