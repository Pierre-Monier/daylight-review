// ABOUTME: parses GitLab CODEOWNERS file format into sections with rules and owner lists
// ABOUTME: strips @ prefix from usernames; rule owners fall back to section defaults if absent
package ownership

import "strings"

type Section struct {
	Name          string
	DefaultOwners []string
	Rules         []Rule
}

type Rule struct {
	Pattern string
	Owners  []string
}

func Parse(content string) []Section {
	var sections []Section
	var current *Section

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			s := parseSection(line)
			sections = append(sections, s)
			current = &sections[len(sections)-1]
			continue
		}
		if current != nil && strings.HasPrefix(line, "/") {
			current.Rules = append(current.Rules, parseRule(line, current.DefaultOwners))
		}
	}
	return sections
}

func parseSection(line string) Section {
	end := strings.Index(line, "]")
	name := line[1:end]
	rest := line[end+1:]
	if strings.HasPrefix(rest, "[") {
		rest = rest[strings.Index(rest, "]")+1:]
	}
	return Section{
		Name:          name,
		DefaultOwners: parseOwners(strings.TrimSpace(rest)),
	}
}

func parseRule(line string, sectionDefaults []string) Rule {
	parts := strings.Fields(line)
	pattern := parts[0]
	if len(parts) > 1 {
		return Rule{Pattern: pattern, Owners: parseOwners(strings.Join(parts[1:], " "))}
	}
	owners := make([]string, len(sectionDefaults))
	copy(owners, sectionDefaults)
	return Rule{Pattern: pattern, Owners: owners}
}

func parseOwners(s string) []string {
	if s == "" {
		return nil
	}
	fields := strings.Fields(s)
	owners := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.HasPrefix(f, "@") {
			owners = append(owners, f[1:])
		}
	}
	return owners
}
