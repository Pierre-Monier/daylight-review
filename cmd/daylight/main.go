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
		fmt.Fprintln(os.Stderr, "usage: daylight [flags]")
		fmt.Fprintln(os.Stderr, "  -token       GitLab API token (overrides DAYLIGHT_GITLAB_TOKEN)")
		fmt.Fprintln(os.Stderr, "  -project-id  GitLab project ID (overrides CI_PROJECT_ID)")
		fmt.Fprintln(os.Stderr, "  -mr-iid      Merge request IID (overrides CI_MERGE_REQUEST_IID)")
		fmt.Fprintln(os.Stderr, "  -sha         Commit SHA (overrides CI_COMMIT_SHA)")
		fmt.Fprintln(os.Stderr, "  -url         GitLab URL (overrides DAYLIGHT_GITLAB_URL / CI_SERVER_URL)")
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
