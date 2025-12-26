package git

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

// CommitInfo holds parsed commit information from git log.
type CommitInfo struct {
	Hash    string
	Email   string // Author email (used for user lookup)
	Subject string
}

// ListCommits returns commits for a branch in reverse chronological order.
// The branch is sanitized before use. Limit controls max commits returned.
func ListCommits(repoPath, branch string, limit int) ([]CommitInfo, error) {
	branch = SanitizeBranch(branch)
	stdout, stderr, err := Exec(repoPath, "log", "--format=format:%h %ae %s", "--reverse", branch, fmt.Sprintf("--max-count=%d", limit))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list commits: %s", stderr.String())
	}

	lines := strings.Split(stdout.String(), "\n")
	var commits []CommitInfo

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 3 {
			continue
		}
		commits = append(commits, CommitInfo{
			Hash:    parts[0],
			Email:   parts[1],
			Subject: parts[2],
		})
	}

	return commits, nil
}

// LatestCommit returns the most recent commit on a branch.
// Returns nil if the branch has no commits.
func LatestCommit(repoPath, branch string) (*CommitInfo, error) {
	commits, err := ListCommits(repoPath, branch, 1)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, nil
	}
	return &commits[0], nil
}

// IsEmpty returns true if the branch has no commits.
func IsEmpty(repoPath, branch string) bool {
	_, err := ListCommits(repoPath, branch, 1)
	return err != nil
}
