package metrics

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
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

// Collect gathers running docker containers and uses native go-git to fetch git repository statuses.
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
				safeRepoName := strings.ReplaceAll(repoName, "-", "")
				if strings.HasPrefix(s.Name, repoName+"-") || strings.HasPrefix(s.Name, safeRepoName+"-") {
					repoServices = append(repoServices, s)
				} else if repoName == "traefik" && strings.Contains(s.Name, "traefik") {
					repoServices = append(repoServices, s)
				}
			}

			if len(repoServices) == 0 {
				continue
			}

			rm := RepoMetrics{
				Name:     repoName,
				Services: repoServices,
				Commits:  "0",
			}

			r, err := git.PlainOpen(repoPath)
			if err != nil {
				continue
			}

			head, err := r.Head()
			if err == nil {
				rm.Branch = head.Name().Short()
				
				since := time.Now().Add(-24 * time.Hour)
				cIter, err := r.Log(&git.LogOptions{From: head.Hash(), Since: &since})
				count := 0
				if err == nil {
					cIter.ForEach(func(c *object.Commit) error {
						count++
						return nil
					})
					rm.Commits = strconv.Itoa(count)
				}
			}

			w, err := r.Worktree()
			if err == nil {
				status, err := w.Status()
				if err == nil && !status.IsClean() {
					rm.Dirty = true
				}
			}

			data.Repos = append(data.Repos, rm)
		}
	}

	return data
}
