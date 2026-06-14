// ABOUTME: implements SelectionStrategy by picking a uniformly random candidate excluding the author
// ABOUTME: filters out the author from eligible candidates before selecting
package selection

import "math/rand"

type RandomStrategy struct{}

func (RandomStrategy) Select(candidates []string, exclude string) (string, error) {
	eligible := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c != exclude {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return "", ErrNoEligibleReviewer
	}
	return eligible[rand.Intn(len(eligible))], nil
}
