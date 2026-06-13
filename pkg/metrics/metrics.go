package metrics

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ServiceInfo struct {
	Name   string
	Image  string
	Status string
}

type RepoMetrics struct {
	Name     string
	Branch   string
	Dirty    bool
	Commits  string
	Services []ServiceInfo
}

type DashboardData struct {
	Repos []RepoMetrics
}

func Collect(ctx context.Context, workDir string) DashboardData {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var data DashboardData

	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}|{{.Image}}|{{.Status}}")
	out, err := cmd.Output()
	var allServices []ServiceInfo
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, l := range lines {
			parts := strings.Split(l, "|")
			if len(parts) >= 3 {
				allServices = append(allServices, ServiceInfo{
					Name:   parts[0],
					Image:  parts[1],
					Status: strings.Join(parts[2:], "|"),
				})
			}
		}
	}

	dirs, err := filepath.Glob(filepath.Join(workDir, "*", ".git"))
	if err == nil {
		for _, gitDir := range dirs {
			repoPath := filepath.Dir(gitDir)
			repoName := filepath.Base(repoPath)

			var repoServices []ServiceInfo
			for _, s := range allServices {
				// Check if container name is related to the repo
				safeRepoName := strings.ReplaceAll(repoName, "-", "") // docker compose sometimes strips hyphens
				if strings.HasPrefix(s.Name, repoName+"-") || strings.HasPrefix(s.Name, safeRepoName+"-") {
					repoServices = append(repoServices, s)
				} else if repoName == "traefik" && strings.Contains(s.Name, "traefik") {
					repoServices = append(repoServices, s)
				}
			}

			// If a repository does not have a running service, do not include it.
			if len(repoServices) == 0 {
				continue
			}

			rm := RepoMetrics{
				Name:     repoName,
				Services: repoServices,
				Commits:  "0",
			}

			cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
			branchOut, err := cmd.Output()
			if err == nil {
				rm.Branch = strings.TrimSpace(string(branchOut))
			}

			cmd = exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain")
			statusOut, err := cmd.Output()
			if err == nil && len(strings.TrimSpace(string(statusOut))) > 0 {
				rm.Dirty = true
			}

			// Correct format for 24h: "1 day ago"
			cmd = exec.CommandContext(ctx, "git", "-C", repoPath, "rev-list", "--count", "--since=1 day ago", "HEAD")
			commitsOut, err := cmd.Output()
			if err == nil {
				commitsStr := strings.TrimSpace(string(commitsOut))
				if commitsStr != "" {
					rm.Commits = commitsStr
				}
			}

			data.Repos = append(data.Repos, rm)
		}
	}

	return data
}
