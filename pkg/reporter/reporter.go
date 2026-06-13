package reporter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"transparent/pkg/state"
)

// GenerateMarkdown creates the markdown representation of the history.
func GenerateMarkdown(events []state.Event) string {
	var buf bytes.Buffer
	buf.WriteString("# Transparent Uptime\n\n")
	buf.WriteString("This repository tracks the historic uptime of my local environment.\n\n")
	buf.WriteString("| Timestamp (UTC) | Status | Notes |\n")
	buf.WriteString("| --- | --- | --- |\n")
	
	// We might only want to show the last N events or all of them.
	// For now, let's reverse them so newest is on top.
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		buf.WriteString(fmt.Sprintf("| %s | %s | %s |\n", e.Timestamp.Format(time.RFC3339), e.Status, e.Message))
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
	// We ignore push errors because network might be offline
	cmd.Run()

	return nil
}
