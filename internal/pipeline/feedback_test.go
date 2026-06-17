// ABOUTME: tests for the one-time feedback note body and visibility
package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeedbackNote_ContainsURLAndIsConfidential(t *testing.T) {
	body, confidential := feedbackNote()

	assert.Contains(t, body, feedbackURL)
	assert.True(t, confidential)
}
