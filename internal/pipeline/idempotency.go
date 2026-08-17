// ABOUTME: formats and parses the internal MR note that records processed runs
// ABOUTME: note body is "daylight:processed " followed by JSON {sha, assignments}
package pipeline

import (
	"encoding/json"
	"strings"
)

const notePrefix = "daylight:processed "

type noteState struct {
	SHA         string              `json:"sha"`
	Assignments map[string][]string `json:"assignments"`
	Ooo         []string            `json:"ooo,omitempty"`
}

// FormatNote encodes the processed commit SHA, the scope→reviewers map, and the
// out-of-office usernames as a note body.
func FormatNote(sha string, assignments map[string][]string, ooo []string) string {
	data, _ := json.Marshal(noteState{SHA: sha, Assignments: assignments, Ooo: ooo})
	return notePrefix + string(data)
}

// ParseNote decodes a note body produced by FormatNote. ok is false for any body that is
// not a daylight note or whose SHA is missing. Notes written before OOO support decode
// with an empty ooo list.
func ParseNote(body string) (sha string, assignments map[string][]string, ooo []string, ok bool) {
	if !strings.HasPrefix(body, notePrefix) {
		return "", nil, nil, false
	}
	var state noteState
	if err := json.Unmarshal([]byte(body[len(notePrefix):]), &state); err != nil {
		return "", nil, nil, false
	}
	if state.SHA == "" {
		return "", nil, nil, false
	}
	return state.SHA, state.Assignments, state.Ooo, true
}
