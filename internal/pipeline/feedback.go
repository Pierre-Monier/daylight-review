// ABOUTME: builds the one-time feedback note inviting MR teams to send feedback
// ABOUTME: owns the feedback form URL, the note wording, and the note's visibility
package pipeline

import "fmt"

// feedbackURL is the destination of the feedback link in the MR note.
const feedbackURL = "https://PLACEHOLDER-FORM-URL" // TODO: real form URL

// feedbackNote returns the body and visibility of the one-time feedback note
// posted on the first run against a merge request.
func feedbackNote() (body string, confidential bool) {
	body = fmt.Sprintf("🌅 Reviewers on this MR were assigned by **Daylight**. "+
		"Hit a bug or have feedback? [Let us know](%s).", feedbackURL)
	return body, true
}
