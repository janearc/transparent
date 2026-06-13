package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type ServiceInfo struct {
	Name   string
	Image  string
	Status string
}

type RepoMetrics struct {
	Name             string
	Branch           string
	Dirty            bool
	Commits          int
	RecentPRs        int
	Unpushed         int
	OpenPRs          int
	RemoteURL        string
	Services         []ServiceInfo
	CommitTimestamps []time.Time
}

type DashboardData struct {
	Repos []RepoMetrics
}

// Collect gathers running docker containers and uses native go-git to fetch git repository statuses.
func Collect(ctx context.Context, workDir string) DashboardData {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var data DashboardData

	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", "{{.Names}}|{{.Image}}|{{.Status}}")
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
				Commits:  0,
			}

			r, err := git.PlainOpen(repoPath)
			if err != nil {
				continue
			}

			head, err := r.Head()
			if err == nil {
				rm.Branch = head.Name().Short()

				remotes, _ := r.Remotes()
				for _, rem := range remotes {
					if rem.Config().Name == "origin" && len(rem.Config().URLs) > 0 {
						url := rem.Config().URLs[0]
						if strings.HasPrefix(url, "git@github.com:") {
							url = strings.Replace(url, "git@github.com:", "https://github.com/", 1)
							url = strings.TrimSuffix(url, ".git")
						} else if strings.HasPrefix(url, "https://github.com/") {
							url = strings.TrimSuffix(url, ".git")
						}
						rm.RemoteURL = url
						break
					}
				}

				cIter, err := r.Log(&git.LogOptions{From: head.Hash()})
				count := 0
				prCount := 0
				cutoff := time.Now().Add(-7 * 24 * time.Hour)
				var timestamps []time.Time
				if err == nil {
					cIter.ForEach(func(c *object.Commit) error {
						count++
						if c.Committer.When.After(cutoff) {
							timestamps = append(timestamps, c.Committer.When)
							if len(c.ParentHashes) > 1 {
								prCount++
							}
						}
						return nil
					})
					rm.Commits = count
					rm.RecentPRs = prCount
					rm.CommitTimestamps = timestamps
				}

				// Unpushed commits
				unpushed := 0
				if originRef, err := r.Reference(plumbing.ReferenceName(fmt.Sprintf("refs/remotes/origin/%s", rm.Branch)), true); err == nil {
					cIter2, _ := r.Log(&git.LogOptions{From: head.Hash()})
					cIter2.ForEach(func(c *object.Commit) error {
						if c.Hash == originRef.Hash() {
							return fmt.Errorf("stop")
						}
						unpushed++
						return nil
					})
					rm.Unpushed = unpushed
				} else {
					rm.Unpushed = 0
				}
			}

			if rm.RemoteURL != "" {
				rm.OpenPRs = fetchOpenPRs(rm.RemoteURL)
			} else {
				rm.OpenPRs = -1
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

func fetchOpenPRs(remoteURL string) int {
	if !strings.HasPrefix(remoteURL, "https://github.com/") {
		return -1
	}
	parts := strings.Split(strings.TrimPrefix(remoteURL, "https://github.com/"), "/")
	if len(parts) < 2 {
		return -1
	}
	owner := parts[0]
	repo := parts[1]

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=open", owner, repo)
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return -1
	}
	req.Header.Set("User-Agent", "transparent-daemon")

	resp, err := client.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return -1
	}

	var pulls []struct {
		Number int `json:"number"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pulls); err != nil {
		return -1
	}
	return len(pulls)
}
