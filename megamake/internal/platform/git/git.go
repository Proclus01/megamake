package git

import (
	"bytes"
	"os/exec"
	"strings"
)

// ChangedFilesSince returns git diff name-only for ref..HEAD, or empty if git is unavailable or not a repo.
func ChangedFilesSince(root string, ref string) []string {
	return runNameOnly(root, []string{"diff", "--name-only", ref + "..HEAD"})
}

// ChangedFilesInRange returns git diff name-only for the specified range (A..B or A...B), or empty if unavailable.
func ChangedFilesInRange(root string, rng string) []string {
	return runNameOnly(root, []string{"diff", "--name-only", rng})
}

func runNameOnly(root string, args []string) []string {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	if !isGitWorkTree(gitPath, root) {
		return nil
	}

	cmd := exec.Command(gitPath, args...)
	cmd.Dir = root

	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil
	}

	lines := strings.Split(out.String(), "\n")
	var res []string
	for _, ln := range lines {
		s := strings.TrimSpace(ln)
		if s == "" {
			continue
		}
		// Normalize to POSIX relpaths.
		s = strings.ReplaceAll(s, "\\", "/")
		res = append(res, s)
	}
	return uniqueSorted(res)
}

func isGitWorkTree(gitPath string, root string) bool {
	cmd := exec.Command(gitPath, "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	_ = cmd.Run()
	return cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 0
}

func uniqueSorted(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range items {
		if strings.TrimSpace(s) == "" {
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	// Insertion sort for small lists (keeps imports minimal).
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j-1] > out[j] {
			out[j-1], out[j] = out[j], out[j-1]
			j--
		}
	}
	return out
}
