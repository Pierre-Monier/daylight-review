// ABOUTME: orchestrates the reviewer assignment pipeline for a single merge request
// ABOUTME: checks idempotency, resolves ownership, selects reviewers, and records the result
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/daylight-review/daylight/internal/config"
	"github.com/daylight-review/daylight/internal/gitlab"
	"github.com/daylight-review/daylight/internal/ownership"
	"github.com/daylight-review/daylight/internal/selection"
)

func Run(ctx context.Context, cfg config.Config, gl gitlab.GitLabClient, strategy selection.SelectionStrategy) error {
	if cfg.IsDraft {
		log.Println("MR is a draft, skipping")
		return nil
	}

	notes, err := gl.MRNotes(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR notes: %w", err)
	}
	for _, note := range notes {
		sha, _, ok := ParseNote(note.Body)
		if ok && sha == cfg.CommitSHA {
			log.Println("already processed this commit, skipping")
			return nil
		}
	}

	files, author, err := gl.MRChanges(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR changes: %w", err)
	}
	log.Printf("author: %s, changed files: %d", author, len(files))
	hash := filesHash(files)

	for _, note := range notes {
		_, fh, ok := ParseNote(note.Body)
		if ok && fh == hash {
			log.Println("already processed this file set, skipping")
			return nil
		}
	}

	content, err := gl.CODEOWNERSContent(ctx, cfg.ProjectID, cfg.CommitSHA)
	if err != nil {
		return fmt.Errorf("fetch CODEOWNERS: %w", err)
	}

	sections := ownership.Parse(content)
	reviewers := ResolveAndSelect(files, sections, author, strategy, log.Printf)
	if len(reviewers) == 0 {
		return nil
	}

	log.Printf("=== RESULT ===")
	log.Printf("assigning reviewers: %s", strings.Join(reviewers, ", "))

	if err := gl.SetReviewers(ctx, cfg.ProjectID, cfg.MRIID, reviewers); err != nil {
		return fmt.Errorf("set reviewers: %w", err)
	}

	if err := gl.PostInternalNote(ctx, cfg.ProjectID, cfg.MRIID, FormatNote(cfg.CommitSHA, hash)); err != nil {
		return fmt.Errorf("post note: %w", err)
	}

	log.Println("done")
	return nil
}

// ResolveAndSelect resolves owners for the changed files, selects reviewers with detailed
// per-step output via printf, and returns the selected usernames.
// Returns nil if no owners are found or the reviewer pool is exhausted.
func ResolveAndSelect(files []string, sections []ownership.Section, author string, strategy selection.SelectionStrategy, printf func(string, ...any)) []string {
	printf("=== FILE MATCHING ===")
	printf("author: %q (excluded from selection)", author)
	printf("changed files (%d):", len(files))
	for _, f := range files {
		normalized := ownership.NormalizePath(f)
		matched := false
		for _, s := range sections {
			for _, r := range s.Rules {
				if strings.HasPrefix(normalized, r.Pattern) {
					printf("  %-40s → [%s] rule %s", f, s.Name, r.Pattern)
					matched = true
				}
			}
		}
		if !matched {
			printf("  %-40s → (no match)", f)
		}
	}

	teamCandidates := ownership.Resolve(files, sections)
	if len(teamCandidates) == 0 {
		printf("no owners found for changed files")
		return nil
	}

	sectionCount := make(map[string]int, len(sections))
	for _, s := range sections {
		sectionCount[s.Name] = s.RequiredCount
	}

	printf("=== SELECTION ===")
	selected := make(map[string]bool)
	for team, candidates := range teamCandidates {
		count := sectionCount[team]
		if count == 0 {
			count = 1
		}
		printf("[%s] — pool: %s, need: %d", team, strings.Join(candidates, ", "), count)
		for i := 0; i < count; i++ {
			remaining := make([]string, 0, len(candidates))
			for _, c := range candidates {
				if !selected[c] {
					remaining = append(remaining, c)
				}
			}
			filtered := make([]string, 0, len(remaining))
			for _, c := range remaining {
				if c != author {
					filtered = append(filtered, c)
				}
			}
			if len(filtered) < len(remaining) {
				printf("  round %d: %s excluded (author), remaining pool: %s", i+1, author, strings.Join(filtered, ", "))
			}
			reviewer, err := strategy.Select(remaining, author)
			if err != nil {
				printf("  round %d: pool exhausted, no reviewer assigned", i+1)
				break
			}
			selected[reviewer] = true
			printf("  round %d: selected %s", i+1, reviewer)
		}
	}

	if len(selected) == 0 {
		return nil
	}
	reviewers := make([]string, 0, len(selected))
	for r := range selected {
		reviewers = append(reviewers, r)
	}
	return reviewers
}

func filesHash(files []string) string {
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(h[:])
}
