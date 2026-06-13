package reporter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"transparent/pkg/metrics"
	"transparent/pkg/state"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// GenerateMarkdown creates the markdown representation of the dashboard.
func GenerateMarkdown(events []state.Event, metricsData metrics.DashboardData) string {
	var buf bytes.Buffer
	buf.WriteString("# Local Environment Dashboard\n\n")

	outages := 0
	for _, e := range events {
		if e.Status == state.StatusAsleep || e.Status == state.StatusOffline {
			outages++
		}
	}
	
	statusText := "Offline"
	if len(events) > 0 {
		if events[len(events)-1].Status == state.StatusOnline {
			statusText = "Online"
		}
	}

	buf.WriteString(fmt.Sprintf("**Host Connectivity:** %s (%d historical outages recorded)\n\n", statusText, outages))
	
	buf.WriteString("## Active Services & Codebases\n\n")

	if len(metricsData.Repos) == 0 {
		buf.WriteString("_No active local services detected._\n")
	}

	for _, repo := range metricsData.Repos {
		buf.WriteString(fmt.Sprintf("### 🐳 `%s`\n", repo.Name))
		
		dirtyStr := ""
		if repo.Dirty {
			dirtyStr = " [dirty]"
		}
		
		buf.WriteString(fmt.Sprintf("* **Git:** branch `%s`%s, %s commits today\n", repo.Branch, dirtyStr, repo.Commits))
		buf.WriteString("* **Services:**\n")
		
		for _, s := range repo.Services {
			buf.WriteString(fmt.Sprintf("  - `%s` (`%s`) - **%s**\n", s.Name, s.Image, s.Status))
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

// CommitAndPush writes the markdown, commits it, and pushes to git natively.
func CommitAndPush(ctx context.Context, repoPath string, markdownPath string, markdownContent string) error {
	if err := os.WriteFile(markdownPath, []byte(markdownContent), 0644); err != nil {
		return err
	}

	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("failed to access worktree: %w", err)
	}

	_, err = w.Add("uptime.md")
	if err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return err
	}

	if status.IsClean() {
		return nil
	}

	_, err = w.Commit("chore: update uptime dashboard", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Transparent Daemon",
			Email: "transparent@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	var auth *ssh.PublicKeys
	auth, err = ssh.NewPublicKeysFromFile("git", "/root/.ssh/id_ed25519", "")
	if err != nil {
		auth, _ = ssh.NewPublicKeysFromFile("git", "/root/.ssh/id_rsa", "")
	}

	err = r.PushContext(ctx, &git.PushOptions{
		Auth: auth,
	})
	if err != nil && err != git.ErrNonFastForwardUpdate {
		// Suppress network errors
		return nil
	}

	return nil
}
