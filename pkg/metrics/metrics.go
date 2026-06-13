package metrics

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DashboardData struct {
	Containers []string
	GitStats   []string
}

// Collect gathers running docker containers and git repository statuses.
func Collect(ctx context.Context, workDir string) DashboardData {
	// Give metrics collection a strict timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var data DashboardData

	// Get Docker containers via local socket
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "- `{{.Names}}` ({{.Image}})")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, l := range lines {
			if l != "" && l != "- `` ()" {
				data.Containers = append(data.Containers, l)
			}
		}
	} else {
		data.Containers = append(data.Containers, "_(Docker daemon unreachable)_")
	}

	// Get Git metrics from workDir by scanning 1 level deep for .git
	dirs, err := filepath.Glob(filepath.Join(workDir, "*", ".git"))
	if err == nil {
		for _, gitDir := range dirs {
			repoPath := filepath.Dir(gitDir)
			repoName := filepath.Base(repoPath)

			// Check if it has any commits
			cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
			branchOut, err := cmd.Output()
			if err != nil {
				continue
			}
			branch := strings.TrimSpace(string(branchOut))
			if branch == "" {
				continue
			}

			// Check dirty state
			cmd = exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain")
			statusOut, _ := cmd.Output()
			dirty := ""
			if len(strings.TrimSpace(string(statusOut))) > 0 {
				dirty = " [dirty]"
			}

			// Check commits in last 24h
			cmd = exec.CommandContext(ctx, "git", "-C", repoPath, "rev-list", "--count", "--since=24h", "HEAD")
			commitsOut, _ := cmd.Output()
			commits := strings.TrimSpace(string(commitsOut))
			if commits == "" {
				commits = "0"
			}

			data.GitStats = append(data.GitStats, "- `"+repoName+"` (branch: "+branch+dirty+", 24h commits: "+commits+")")
		}
	} else {
		data.GitStats = append(data.GitStats, "_(Could not scan git repositories)_")
	}

	return data
}
