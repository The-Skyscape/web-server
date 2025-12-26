package git

import (
	"bytes"
	"os/exec"
)

// Exec runs a git command in the specified repository path.
// Returns stdout, stderr buffers and any error from execution.
func Exec(repoPath string, args ...string) (stdout, stderr bytes.Buffer, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	return stdout, stderr, cmd.Run()
}
