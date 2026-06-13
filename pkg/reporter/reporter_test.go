package reporter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"transparent/pkg/metrics"
	"transparent/pkg/state"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestGenerateDashboard(t *testing.T) {
	reportDir := filepath.Join(t.TempDir(), "report")
	storePath := filepath.Join(t.TempDir(), "store.json")
	store, err := state.NewStore(storePath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Add some data to store
	store.RecordServiceMetrics("test-repo/test-svc", true, 10)
	store.RecordServiceMetrics("test-repo/test-svc-down", false, 10)

	events := []state.Event{
		{Timestamp: time.Now().Add(-2 * time.Hour), Status: state.StatusOffline},
		{Timestamp: time.Now().Add(-1 * time.Hour), Status: state.StatusOnline},
	}

	metricsData := metrics.DashboardData{
		Repos: []metrics.RepoMetrics{
			{
				Name:      "test-repo",
				RemoteURL: "https://github.com/test/repo",
				Branch:    "main",
				Dirty:     true,
				Commits:   42,
				RecentPRs: 3,
				Unpushed:  1,
				OpenPRs:   2,
				Services: []metrics.ServiceInfo{
					{Name: "test-repo/test-svc", Image: "test-img:latest", Status: "Up 2 hours"},
					{Name: "test-repo/test-svc-down", Image: "test-img:latest", Status: "Exited (1) 2 hours ago"},
				},
				CommitTimestamps: []time.Time{
					time.Now(),
					time.Now().Add(-2 * time.Hour),
					time.Now().Add(-24 * time.Hour),
					// 4 items => #40c463
					time.Now().Add(-48 * time.Hour),
					time.Now().Add(-48 * time.Hour),
					time.Now().Add(-48 * time.Hour),
					time.Now().Add(-48 * time.Hour),
					// 12 items => #216e39
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					time.Now().Add(-72 * time.Hour),
					// 7 items => #30a14e
					time.Now().Add(-96 * time.Hour),
					time.Now().Add(-96 * time.Hour),
					time.Now().Add(-96 * time.Hour),
					time.Now().Add(-96 * time.Hour),
					time.Now().Add(-96 * time.Hour),
					time.Now().Add(-96 * time.Hour),
					time.Now().Add(-96 * time.Hour),
				},
			},
			{
				Name: "empty-repo",
			},
		},
	}

	err = GenerateDashboard(reportDir, events, metricsData, store)
	if err != nil {
		t.Fatalf("GenerateDashboard failed: %v", err)
	}

	expectedFiles := []string{
		"index.html",
		"uptime.md",
		"data.json",
		"data.xml",
		"test-repo_test-svc_uptime.svg",
		"test-repo_test-svc_churn.svg",
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(reportDir, f)); os.IsNotExist(err) {
			t.Errorf("expected file %s to be created", f)
		}
	}
}

func TestGenerateDashboardMkdirError(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "file")
	os.WriteFile(tmpFile, []byte("test"), 0644)
	err := GenerateDashboard(tmpFile, nil, metrics.DashboardData{}, nil)
	if err == nil {
		t.Errorf("expected error when reportDir cannot be created")
	}
}

func TestGenerateDashboardEmpty(t *testing.T) {
	reportDir := filepath.Join(t.TempDir(), "report_empty")
	storePath := filepath.Join(t.TempDir(), "store.json")
	store, _ := state.NewStore(storePath)

	err := GenerateDashboard(reportDir, []state.Event{}, metrics.DashboardData{}, store)
	if err != nil {
		t.Fatalf("GenerateDashboard failed: %v", err)
	}
}

func TestCommitDashboard(t *testing.T) {
	repoDir := t.TempDir()

	r, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	w, err := r.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	reportDir := filepath.Join(repoDir, "REPORT")
	os.MkdirAll(reportDir, 0755)
	os.WriteFile(filepath.Join(reportDir, "index.html"), []byte("test"), 0644)

	ctx := context.Background()

	err = CommitDashboard(ctx, repoDir)
	if err != nil {
		t.Fatalf("CommitDashboard (first) failed: %v", err)
	}

	// Call again without changes to test IsClean() early exit
	err = CommitDashboard(ctx, repoDir)
	if err != nil {
		t.Fatalf("CommitDashboard (no changes) failed: %v", err)
	}

	// Create an initial commit to make the tree have parents
	os.WriteFile(filepath.Join(repoDir, "dummy.txt"), []byte("dummy"), 0644)
	w.Add("dummy.txt")
	w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "User", Email: "user@local", When: time.Now()},
	})

	os.WriteFile(filepath.Join(reportDir, "index.html"), []byte("test3"), 0644)
	err = CommitDashboard(ctx, repoDir)
	if err != nil {
		t.Fatalf("CommitDashboard (third) failed: %v", err)
	}

	// Triggers soft-reset branch
	os.WriteFile(filepath.Join(reportDir, "index.html"), []byte("test4"), 0644)
	err = CommitDashboard(ctx, repoDir)
	if err != nil {
		t.Fatalf("CommitDashboard (fourth) failed: %v", err)
	}
}

func TestCommitDashboardErrors(t *testing.T) {
	err := CommitDashboard(context.Background(), "/non/existent/dir")
	if err == nil {
		t.Errorf("expected error for non-existent repo")
	}
}

func TestGenerateSVGGridFallback(t *testing.T) {
	colors := []string{"#fff"}
	out := generateSVGGrid(colors)
	if out == "" {
		t.Errorf("generateSVGGrid returned empty")
	}
}
