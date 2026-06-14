// ABOUTME: orchestrates the reviewer assignment pipeline for a single merge request
// ABOUTME: checks idempotency, resolves ownership, selects reviewers, and records the result
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/ownership"
	"github.com/daylight-review/daylight/internal/selection"
)

func Run(ctx context.Context, cfg config.Config, gl gitlab.GitLabClient, strategy selection.SelectionStrategy) error {
	if cfg.IsDraft {
		return nil
	}

	notes, err := gl.MRNotes(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR notes: %w", err)
	}
	for _, note := range notes {
		sha, _, ok := ParseNote(note.Body)
		if ok && sha == cfg.CommitSHA {
			return nil
		}
	}

	files, author, err := gl.MRChanges(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR changes: %w", err)
	}
	hash := filesHash(files)

	for _, note := range notes {
		_, fh, ok := ParseNote(note.Body)
		if ok && fh == hash {
			return nil
		}
	}

	content, err := gl.CODEOWNERSContent(ctx, cfg.ProjectID, cfg.CommitSHA)
	if err != nil {
		return fmt.Errorf("fetch CODEOWNERS: %w", err)
	}

	sections := ownership.Parse(content)
	teamCandidates := ownership.Resolve(files, sections)
	if len(teamCandidates) == 0 {
		return nil
	}

	sectionCount := make(map[string]int, len(sections))
	for _, s := range sections {
		sectionCount[s.Name] = s.RequiredCount
	}

	selected := make(map[string]bool)
	for team, candidates := range teamCandidates {
		count := sectionCount[team]
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			remaining := make([]string, 0, len(candidates))
			for _, c := range candidates {
				if !selected[c] {
					remaining = append(remaining, c)
				}
			}
			reviewer, err := strategy.Select(remaining, author)
			if err != nil {
				break
			}
			selected[reviewer] = true
		}
	}
	if len(selected) == 0 {
		return nil
	}

	reviewers := make([]string, 0, len(selected))
	for r := range selected {
		reviewers = append(reviewers, r)
	}

	if err := gl.SetReviewers(ctx, cfg.ProjectID, cfg.MRIID, reviewers); err != nil {
		return fmt.Errorf("set reviewers: %w", err)
	}

	if err := gl.PostInternalNote(ctx, cfg.ProjectID, cfg.MRIID, FormatNote(cfg.CommitSHA, hash)); err != nil {
		return fmt.Errorf("post note: %w", err)
	}

	return nil
}

func filesHash(files []string) string {
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(h[:])
}
