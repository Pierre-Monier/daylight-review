// ABOUTME: formats and parses internal MR notes used to track processed pipeline runs
// ABOUTME: note format: "daylight:processed sha=<sha> files_hash=<hash>"
package pipeline

import (
	"fmt"
	"strings"
)

func FormatNote(sha, filesHash string) string {
	return fmt.Sprintf("daylight:processed sha=%s files_hash=%s", sha, filesHash)
}

func ParseNote(body string) (sha, filesHash string, ok bool) {
	if !strings.HasPrefix(body, "daylight:processed ") {
		return "", "", false
	}
	parts := strings.Fields(body)
	if len(parts) != 3 {
		return "", "", false
	}
	for _, part := range parts[1:] {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return "", "", false
		}
		switch kv[0] {
		case "sha":
			sha = kv[1]
		case "files_hash":
			filesHash = kv[1]
		}
	}
	return sha, filesHash, sha != "" && filesHash != ""
}
