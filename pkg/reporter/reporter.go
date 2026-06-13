package reporter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"transparent/pkg/metrics"
	"transparent/pkg/state"
)

// GenerateMarkdown creates the markdown representation of the dashboard.
func GenerateMarkdown(events []state.Event, metricsData metrics.DashboardData) string {
	var buf bytes.Buffer
	buf.WriteString("# Local Environment Dashboard\n\n")

	// Calculate single-line host uptime summary
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

// CommitAndPush writes the markdown, commits it, and pushes to git.
func CommitAndPush(ctx context.Context, repoPath string, markdownPath string, markdownContent string) error {
	if err := os.WriteFile(markdownPath, []byte(markdownContent), 0644); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "git", "add", markdownPath)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	cmd = exec.CommandContext(ctx, "git", "commit", "-m", "chore: update uptime dashboard")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		// git commit fails if there are no changes, which is fine
		return nil
	}

	cmd = exec.CommandContext(ctx, "git", "push")
	cmd.Dir = repoPath
	cmd.Run()

	return nil
}
