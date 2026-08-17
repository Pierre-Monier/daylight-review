// ABOUTME: parses out-of-office command notes and builds the set of flagged usernames
// ABOUTME: command syntax is "daylight:ooo @username" (the @ is optional)
package pipeline

import (
	"sort"
	"strings"

	"github.com/daylight-review/daylight/internal/gitlab"
)

const oooCommandPrefix = "daylight:ooo"

// ParseOOOCommand extracts the username from a "daylight:ooo @username" note body.
// The leading @ is optional. ok is false for any body that is not an OOO command.
func ParseOOOCommand(body string) (username string, ok bool) {
	fields := strings.Fields(body)
	if len(fields) < 2 || fields[0] != oooCommandPrefix {
		return "", false
	}
	return strings.TrimPrefix(fields[1], "@"), true
}

// requestedOOO returns the sorted, deduped usernames flagged out-of-office across all notes.
func requestedOOO(notes []gitlab.Note) []string {
	set := make(map[string]bool)
	for _, n := range notes {
		if u, ok := ParseOOOCommand(n.Body); ok {
			set[u] = true
		}
	}
	return sortedKeys(set)
}

// unionOOO returns the sorted, deduped union of two username lists.
func unionOOO(a, b []string) []string {
	set := make(map[string]bool, len(a)+len(b))
	for _, u := range a {
		set[u] = true
	}
	for _, u := range b {
		set[u] = true
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
