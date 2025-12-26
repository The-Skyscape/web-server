package git

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

// FileEntry represents a file or directory in the git tree.
type FileEntry struct {
	Path  string
	IsDir bool
}

// ListFiles returns files and directories at the given path in a branch.
// Results are sorted with directories first, then alphabetically.
func ListFiles(repoPath, branch, path string) ([]FileEntry, error) {
	branch = SanitizeBranch(branch)
	stdout, _, err := Exec(repoPath, "ls-tree", branch, filepath.Join(".", path)+"/")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list files: %s @ %s", branch, path)
	}

	var files []FileEntry
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		if parts := strings.Fields(line); len(parts) >= 4 {
			files = append(files, FileEntry{
				Path:  parts[3],
				IsDir: parts[1] == "tree",
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir && !files[j].IsDir {
			return true
		}
		if !files[i].IsDir && files[j].IsDir {
			return false
		}
		return files[i].Path < files[j].Path
	})

	return files, nil
}

// IsDir checks if a path is a directory in the given branch.
// Returns true for empty path or ".".
func IsDir(repoPath, branch, path string) (bool, error) {
	branch = SanitizeBranch(branch)
	if path == "" || path == "." {
		return true, nil
	}

	stdout, _, err := Exec(repoPath, "ls-tree", branch, filepath.Join(".", path))
	if err != nil {
		return false, errors.Wrap(err, "failed to list files")
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return false, errors.New("no such file or directory")
	}

	parts := strings.Fields(output)
	return parts[1] == "tree", nil
}

// FileContent holds the content of a file and whether it's binary.
type FileContent struct {
	Content  string
	IsBinary bool
}

// ReadFile reads the content of a file at the given path and branch.
func ReadFile(repoPath, branch, path string) (*FileContent, error) {
	branch = SanitizeBranch(branch)
	stdout, _, err := Exec(repoPath, "show", fmt.Sprintf("%s:%s", branch, path))
	if err != nil {
		return nil, errors.Wrap(err, "failed to show file")
	}

	content := stdout.String()
	return &FileContent{
		Content:  content,
		IsBinary: strings.Contains(content, "\x00"),
	}, nil
}
