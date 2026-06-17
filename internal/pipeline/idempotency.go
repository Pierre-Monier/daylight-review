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
}

// FormatNote encodes the processed commit SHA and the scope→reviewers map as a note body.
func FormatNote(sha string, assignments map[string][]string) string {
	data, _ := json.Marshal(noteState{SHA: sha, Assignments: assignments})
	return notePrefix + string(data)
}

// ParseNote decodes a note body produced by FormatNote. ok is false for any body that is
// not a daylight note or whose SHA is missing.
func ParseNote(body string) (sha string, assignments map[string][]string, ok bool) {
	if !strings.HasPrefix(body, notePrefix) {
		return "", nil, false
	}
	var state noteState
	if err := json.Unmarshal([]byte(body[len(notePrefix):]), &state); err != nil {
		return "", nil, false
	}
	if state.SHA == "" {
		return "", nil, false
	}
	return state.SHA, state.Assignments, true
}
