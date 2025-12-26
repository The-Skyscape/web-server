package git

import (
	"regexp"
	"strings"
)

// SanitizeBranch validates and sanitizes branch names to prevent path traversal
// and unauthorized access to git refs. Returns "main" as default for invalid branches.
func SanitizeBranch(branch string) string {
	if branch == "" {
		return "main"
	}

	// Only allow alphanumeric, dash, underscore, and forward slash
	validBranchRegex := regexp.MustCompile(`^[a-zA-Z0-9/_-]+$`)
	if !validBranchRegex.MatchString(branch) {
		return "main"
	}

	// Disallow dangerous patterns that could access unauthorized refs
	dangerous := []string{
		"refs/", "HEAD~", "HEAD^", "@{",
		"..", "//", "stash",
	}

	for _, pattern := range dangerous {
		if strings.Contains(branch, pattern) {
			return "main"
		}
	}

	return branch
}
