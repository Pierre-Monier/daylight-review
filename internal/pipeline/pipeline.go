// ABOUTME: orchestrates the reviewer assignment pipeline for a single merge request
// ABOUTME: checks idempotency, resolves ownership, selects reviewers, and records the result
package pipeline

import (
	"context"
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

	var prevSHA string
	var prevAssignments map[string][]string
	var prevOoo []string
	// Notes are newest-first, so the first daylight note is the current state.
	for _, note := range notes {
		if sha, assignments, ooo, ok := ParseNote(note.Body); ok {
			prevSHA, prevAssignments, prevOoo = sha, assignments, ooo
			break
		}
	}
	if prevSHA == cfg.CommitSHA {
		log.Println("already processed this commit, skipping")
		return nil
	}

	files, author, err := gl.MRChanges(ctx, cfg.ProjectID, cfg.MRIID)
	if err != nil {
		return fmt.Errorf("fetch MR changes: %w", err)
	}
	log.Printf("author: %s, changed files: %d", author, len(files))

	content, err := gl.CODEOWNERSContent(ctx, cfg.ProjectID, cfg.CommitSHA)
	if err != nil {
		return fmt.Errorf("fetch CODEOWNERS: %w", err)
	}
	sections := ownership.Parse(content)

	assignments := DiffAndSelect(files, sections, author, prevAssignments, strategy, log.Printf)
	newReviewers := Reviewers(assignments)
	prevReviewers := Reviewers(prevAssignments)

	if len(newReviewers) == 0 && len(prevReviewers) == 0 {
		log.Println("no reviewers to assign")
		return nil
	}

	log.Printf("=== RESULT ===")
	if equalStrings(newReviewers, prevReviewers) {
		log.Printf("reviewers unchanged: %s", strings.Join(newReviewers, ", "))
	} else {
		log.Printf("assigning reviewers: %s", strings.Join(newReviewers, ", "))
		if err := gl.SetReviewers(ctx, cfg.ProjectID, cfg.MRIID, newReviewers); err != nil {
			return fmt.Errorf("set reviewers: %w", err)
		}
	}

	if err := gl.PostNote(ctx, cfg.ProjectID, cfg.MRIID, FormatNote(cfg.CommitSHA, assignments, prevOoo), true); err != nil {
		return fmt.Errorf("post note: %w", err)
	}

	if prevSHA == "" {
		body, confidential := feedbackNote()
		if err := gl.PostNote(ctx, cfg.ProjectID, cfg.MRIID, body, confidential); err != nil {
			log.Printf("warning: post feedback note: %v", err)
		}
	}

	log.Println("done")
	return nil
}

// equalStrings reports whether two string slices are element-wise equal. Callers pass
// Reviewers output, which is sorted, so element-wise comparison is a set comparison.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DiffAndSelect computes the new per-scope reviewer assignments. Scopes matched by the
// changed files are diffed against the previously-stored assignments: a scope present in
// both keeps its stored reviewers verbatim; a newly-matched scope gets freshly-selected
// reviewers (count from the section, excluding the author and anyone already assigned);
// a scope no longer matched is dropped. Emits per-step detail via printf.
func DiffAndSelect(files []string, sections []ownership.Section, author string, previous map[string][]string, strategy selection.SelectionStrategy, printf func(string, ...any)) map[string][]string {
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

	currentPools := ownership.Resolve(files, sections)

	sectionCount := make(map[string]int, len(sections))
	for _, s := range sections {
		sectionCount[s.Name] = s.RequiredCount
	}

	result := make(map[string][]string)
	assigned := make(map[string]bool)

	printf("=== SCOPE DIFF ===")
	for scope := range currentPools {
		if prev, ok := previous[scope]; ok {
			result[scope] = prev
			for _, r := range prev {
				assigned[r] = true
			}
			printf("[%s] kept: %s", scope, strings.Join(prev, ", "))
		}
	}
	for scope, prev := range previous {
		if _, ok := currentPools[scope]; !ok {
			printf("[%s] removed: %s", scope, strings.Join(prev, ", "))
		}
	}
	for scope, candidates := range currentPools {
		if _, ok := previous[scope]; ok {
			continue
		}
		count := sectionCount[scope]
		if count == 0 {
			count = 1
		}
		printf("[%s] new — pool: %s, need: %d", scope, strings.Join(candidates, ", "), count)
		if selected := selectN(candidates, count, author, assigned, strategy, printf); len(selected) > 0 {
			result[scope] = selected
		}
	}

	return result
}

// selectN selects up to count reviewers from candidates, excluding the author and anyone
// already in the assigned set. Selected reviewers are added to assigned so later scopes
// don't reuse them. Stops early (with fewer reviewers) if the pool is exhausted.
func selectN(candidates []string, count int, author string, assigned map[string]bool, strategy selection.SelectionStrategy, printf func(string, ...any)) []string {
	selected := make([]string, 0, count)
	for i := 0; i < count; i++ {
		remaining := make([]string, 0, len(candidates))
		for _, c := range candidates {
			if !assigned[c] {
				remaining = append(remaining, c)
			}
		}
		reviewer, err := strategy.Select(remaining, author)
		if err != nil {
			printf("  round %d: pool exhausted, no reviewer assigned", i+1)
			break
		}
		assigned[reviewer] = true
		selected = append(selected, reviewer)
		printf("  round %d: selected %s", i+1, reviewer)
	}
	return selected
}

// Reviewers flattens a scope→reviewers map into a sorted, de-duplicated reviewer list.
func Reviewers(assignments map[string][]string) []string {
	set := make(map[string]bool)
	for _, reviewers := range assignments {
		for _, r := range reviewers {
			set[r] = true
		}
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
