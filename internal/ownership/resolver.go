// ABOUTME: maps a list of changed file paths to candidate reviewer pools per owning section
// ABOUTME: uses prefix matching; unions owners from all matched rules within each section
package ownership

import "strings"

// Resolve returns a map of section name to candidate usernames for the given changed files.
// Files with no matching rule are ignored. Returns an empty map if nothing has an owner.
func Resolve(changedFiles []string, sections []Section) map[string][]string {
	seen := make(map[string]map[string]bool)

	for _, file := range changedFiles {
		for _, section := range sections {
			for _, rule := range section.Rules {
				if strings.HasPrefix(file, rule.Pattern) {
					if seen[section.Name] == nil {
						seen[section.Name] = make(map[string]bool)
					}
					for _, owner := range rule.Owners {
						seen[section.Name][owner] = true
					}
				}
			}
		}
	}

	result := make(map[string][]string, len(seen))
	for name, ownerSet := range seen {
		owners := make([]string, 0, len(ownerSet))
		for owner := range ownerSet {
			owners = append(owners, owner)
		}
		result[name] = owners
	}
	return result
}
