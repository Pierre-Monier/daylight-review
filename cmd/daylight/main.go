// ABOUTME: entry point for the daylight CLI — wires config, GitLab client, and pipeline
// ABOUTME: exits with code 1 and prints the error on any failure
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/pipeline"
	"github.com/daylight-review/daylight/internal/selection"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "assign" {
		fmt.Fprintln(os.Stderr, "usage: daylight assign")
		os.Exit(1)
	}

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
