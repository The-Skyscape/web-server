package hosting

import (
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

var (
	unsafeChars    = regexp.MustCompile(`[^a-z0-9_-]+`)
	multipleHyphen = regexp.MustCompile(`-+`)
)

// SanitizeID generates a safe ID from a name.
// Removes dangerous characters to prevent command injection and path traversal.
func SanitizeID(name string) (string, error) {
	id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	id = unsafeChars.ReplaceAllString(id, "")
	id = multipleHyphen.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")

	if id == "" {
		return "", errors.New("name must contain at least one alphanumeric character")
	}

	return id, nil
}

// ValidateID checks if an ID contains only safe characters
func ValidateID(id string) bool {
	return id != "" && !unsafeChars.MatchString(id)
}
