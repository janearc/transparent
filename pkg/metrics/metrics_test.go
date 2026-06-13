package metrics

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type mockTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

func TestFetchOpenPRs(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	tests := []struct {
		name      string
		remoteURL string
		mockResp  *http.Response
		mockErr   error
		want      int
	}{
		{
			name:      "not github",
			remoteURL: "https://gitlab.com/owner/repo",
			want:      -1,
		},
		{
			name:      "malformed github",
			remoteURL: "https://github.com/owner",
			want:      -1,
		},
		{
			name:      "http error",
			remoteURL: "https://github.com/owner/repo",
			mockErr:   fmt.Errorf("network error"),
			want:      -1,
		},
		{
			name:      "non 200 status",
			remoteURL: "https://github.com/owner/repo",
			mockResp: &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString("")),
			},
			want: -1,
		},
		{
			name:      "bad json",
			remoteURL: "https://github.com/owner/repo",
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("invalid json")),
			},
			want: -1,
		},
		{
			name:      "success",
			remoteURL: "https://github.com/owner/repo",
			mockResp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`[{"number":1},{"number":2}]`)),
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockResp != nil || tt.mockErr != nil {
				http.DefaultTransport = &mockTransport{
					RoundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.mockResp, tt.mockErr
					},
				}
			}

			got := fetchOpenPRs(tt.remoteURL)
			if got != tt.want {
				t.Errorf("fetchOpenPRs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCollect(t *testing.T) {
	// Create temp dir
	workDir, err := os.MkdirTemp("", "metrics-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workDir)

	// Create mock docker command
	dockerScript := filepath.Join(workDir, "docker")
	err = os.WriteFile(dockerScript, []byte(`#!/bin/sh
if [ "$1" = "ps" ]; then
	echo "myrepo-web|nginx|Up 2 hours"
	echo "traefik-proxy|traefik|Up 1 hour"
	echo "other-app|node|Exited (1)"
	exit 0
fi
echo "unknown command" >&2
exit 1
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Override PATH to use our mock docker
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", workDir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	// Create a mock git repository "myrepo"
	repoPath := filepath.Join(workDir, "myrepo")
	err = os.MkdirAll(repoPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	r, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatal(err)
	}

	// Add remote
	_, err = r.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"git@github.com:owner/myrepo.git"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add another remote just to cover logic
	_, err = r.CreateRemote(&config.RemoteConfig{
		Name: "other",
		URLs: []string{"https://github.com/owner2/myrepo2"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create commits
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(repoPath, "test.txt"), []byte("hello"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add("test.txt")
	if err != nil {
		t.Fatal(err)
	}

	author := &object.Signature{
		Name:  "Test User",
		Email: "test@example.com",
		When:  time.Now(),
	}

	commit, err := w.Commit("Initial commit", &git.CommitOptions{
		Author: author,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Second commit
	err = os.WriteFile(filepath.Join(repoPath, "test.txt"), []byte("hello 2"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Add("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	commit2, err := w.Commit("Second commit", &git.CommitOptions{Author: author})
	if err != nil {
		t.Fatal(err)
	}

	// Third commit (merge) to cover PR count
	os.WriteFile(filepath.Join(repoPath, "merge.txt"), []byte("merge"), 0644)
	w.Add("merge.txt")

	commit3, err := w.Commit("merge", &git.CommitOptions{
		Author:  author,
		Parents: []plumbing.Hash{commit2, commit},
	})
	_ = commit3
	if err != nil {
		t.Fatal(err)
	}

	// Set origin/master to commit to simulate 2 unpushed commits
	refName := plumbing.ReferenceName("refs/remotes/origin/master")
	ref := plumbing.NewHashReference(refName, commit)
	err = r.Storer.SetReference(ref)
	if err != nil {
		t.Fatal(err)
	}

	// make it dirty
	err = os.WriteFile(filepath.Join(repoPath, "test2.txt"), []byte("dirty"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create "traefik" repo to test its specific condition
	traefikPath := filepath.Join(workDir, "traefik")
	err = os.MkdirAll(traefikPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = git.PlainInit(traefikPath, false)
	if err != nil {
		t.Fatal(err)
	}

	// Create "repo-without-commits" repo
	emptyPath := filepath.Join(workDir, "myrepo-web")
	err = os.MkdirAll(emptyPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = git.PlainInit(emptyPath, false)
	if err != nil {
		t.Fatal(err)
	}

	// Mock HTTP for open PRs
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()
	http.DefaultTransport = &mockTransport{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`[{"number":1}]`)),
			}, nil
		},
	}

	// Run Collect
	ctx := context.Background()
	data := Collect(ctx, workDir)

	if len(data.Repos) < 2 {
		t.Fatalf("expected at least 2 repos (myrepo, traefik), got %d", len(data.Repos))
	}

	var myrepo *RepoMetrics
	var traefikRepo *RepoMetrics

	for i, rep := range data.Repos {
		if rep.Name == "myrepo" {
			myrepo = &data.Repos[i]
		}
		if rep.Name == "traefik" {
			traefikRepo = &data.Repos[i]
		}
	}

	if myrepo == nil {
		t.Fatalf("myrepo not found in results")
	}

	if myrepo.Commits != 3 {
		t.Errorf("expected 3 commits, got %d", myrepo.Commits)
	}
	if !myrepo.Dirty {
		t.Errorf("expected dirty=true, got %v", myrepo.Dirty)
	}
	if myrepo.OpenPRs != 1 {
		t.Errorf("expected 1 open PR, got %d", myrepo.OpenPRs)
	}
	if myrepo.RemoteURL != "https://github.com/owner/myrepo" {
		t.Errorf("unexpected remote url: %s", myrepo.RemoteURL)
	}
	if len(myrepo.Services) != 1 {
		t.Errorf("expected 1 service, got %d", len(myrepo.Services))
	} else if myrepo.Services[0].Name != "myrepo-web" {
		t.Errorf("expected service myrepo-web, got %s", myrepo.Services[0].Name)
	}
	if myrepo.RecentPRs != 1 {
		t.Errorf("expected 1 recent PR, got %d", myrepo.RecentPRs)
	}
	if myrepo.Unpushed != 2 {
		t.Errorf("expected 2 unpushed commits, got %d", myrepo.Unpushed)
	}
	if len(myrepo.CommitTimestamps) != 3 {
		t.Errorf("expected 3 commit timestamps, got %d", len(myrepo.CommitTimestamps))
	}

	if traefikRepo == nil {
		t.Fatalf("traefik repo not found in results")
	}
	if len(traefikRepo.Services) != 1 {
		t.Errorf("expected 1 service for traefik, got %d", len(traefikRepo.Services))
	}
}

func TestCollect_DockerError(t *testing.T) {
	// Create temp dir
	workDir, err := os.MkdirTemp("", "metrics-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workDir)

	// Create a mock docker command that fails
	dockerScript := filepath.Join(workDir, "docker")
	err = os.WriteFile(dockerScript, []byte(`#!/bin/sh
exit 1
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", workDir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	ctx := context.Background()
	data := Collect(ctx, workDir)

	if len(data.Repos) != 0 {
		t.Errorf("expected 0 repos when no docker services, got %d", len(data.Repos))
	}
}
