// ABOUTME: entry point for the daylight CLI — wires config, GitLab client, and pipeline
// ABOUTME: supports "check" subcommand for local dry-run testing of CODEOWNERS logic
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/ownership"
	"github.com/daylight-review/daylight/internal/pipeline"
	"github.com/daylight-review/daylight/internal/selection"
)


func main() {
	if len(os.Args) > 1 && os.Args[1] == "check" {
		runCheck(os.Args[2:])
		return
	}
	runAssign(os.Args[1:])
}

func runAssign(args []string) {
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
		fmt.Fprintln(os.Stderr, "Subcommands:")
		fmt.Fprintln(os.Stderr, "  check   Dry-run: resolve owners from a local CODEOWNERS file")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags override the corresponding env var (useful for local testing):")
		fmt.Fprintln(os.Stderr, "  -token, -project-id, -mr-iid, -sha, -url")
	}
	token := fs.String("token", "", "")
	projectID := fs.String("project-id", "", "")
	mrIID := fs.String("mr-iid", "", "")
	sha := fs.String("sha", "", "")
	url := fs.String("url", "", "")
	_ = fs.Parse(args)

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

func runCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: daylight check -codeowners <file> -author <username> <changed-file> [<changed-file> ...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Resolves owners and selects reviewers without making any API calls.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fmt.Fprintln(os.Stderr, "  -codeowners   path to CODEOWNERS file (default: CODEOWNERS)")
		fmt.Fprintln(os.Stderr, "  -author       MR author username to exclude from selection (optional)")
	}
	codeownersPath := fs.String("codeowners", "CODEOWNERS", "")
	author := fs.String("author", "", "")
	_ = fs.Parse(args)

	changedFiles := fs.Args()
	if len(changedFiles) == 0 {
		fs.Usage()
		os.Exit(1)
	}

	content, err := os.ReadFile(*codeownersPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", *codeownersPath, err)
		os.Exit(1)
	}

	sections := ownership.Parse(string(content))

	printf := func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	assignments, _ := pipeline.DiffAndSelect(changedFiles, sections, *author, nil, nil, selection.RandomStrategy{}, printf)
	reviewers := pipeline.Reviewers(assignments)

	fmt.Println()
	fmt.Println("=== RESULT ===")
	if len(reviewers) == 0 {
		fmt.Println("no reviewers would be assigned")
		return
	}
	fmt.Printf("would assign: %s\n", strings.Join(reviewers, ", "))
}
