// ABOUTME: defines the reviewer selection interface and the sentinel error for empty pools
// ABOUTME: provides the contract for all selection strategies
package selection

import "errors"

// ErrNoEligibleReviewer is returned when the candidate pool is empty or only contains the author.
var ErrNoEligibleReviewer = errors.New("no eligible reviewer")

type SelectionStrategy interface {
	Select(candidates []string, exclude string) (string, error)
}
