package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"transparent/pkg/metrics"
	"transparent/pkg/state"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type XMLDashboard struct {
	XMLName xml.Name  `xml:"dashboard"`
	Repos   []XMLRepo `xml:"repos>repo"`
}

type XMLRepo struct {
	Name      string       `xml:"name"`
	RemoteURL string       `xml:"remoteURL"`
	Branch    string       `xml:"branch"`
	Dirty     bool         `xml:"dirty"`
	Commits   int          `xml:"commits"`
	RecentPRs int          `xml:"recentPRs"`
	Unpushed  int          `xml:"unpushed"`
	OpenPRs   int          `xml:"openPRs"`
	Services  []XMLService `xml:"services>service"`
}

type XMLService struct {
	Name   string `xml:"name"`
	Image  string `xml:"image"`
	Status string `xml:"status"`
}

// GenerateDashboard generates the Markdown, HTML, XML, JSON, and SVGs and writes them to the REPORT dir.
func GenerateDashboard(reportDir string, events []state.Event, metricsData metrics.DashboardData, store *state.Store) error {
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return err
	}

	var mdBuf bytes.Buffer
	var htmlBuf bytes.Buffer

	// HTML Header
	htmlBuf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Transparent Dashboard</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;600&display=swap" rel="stylesheet">
    <style>
        body { background-color: #0d1117; color: #c9d1d9; font-family: 'Inter', sans-serif; margin: 0; padding: 40px; }
        .container { max-width: 900px; margin: 0 auto; background: rgba(22, 27, 34, 0.7); backdrop-filter: blur(10px); border-radius: 16px; padding: 30px; box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3); border: 1px solid rgba(255, 255, 255, 0.1); }
        h1 { color: #f0f6fc; font-weight: 600; margin-top: 0; }
        h2 { color: #58a6ff; font-weight: 400; margin-top: 30px; border-bottom: 1px solid #21262d; padding-bottom: 10px; }
        h3 { color: #f0f6fc; margin-top: 0; }
        .repo-card { background: #161b22; border-radius: 8px; padding: 20px; margin-bottom: 20px; border: 1px solid #30363d; transition: transform 0.2s ease; }
        .repo-card:hover { transform: translateY(-2px); border-color: #8b949e; }
        .service-list { list-style: none; padding: 0; margin: 0; }
        .service-item { background: #0d1117; margin-bottom: 15px; padding: 15px; border-radius: 6px; border: 1px solid #21262d; }
        .graphs { display: flex; gap: 20px; margin-top: 15px; flex-wrap: wrap; }
        .graph-box { flex: 1; min-width: 250px; background: rgba(255, 255, 255, 0.02); padding: 12px; border-radius: 8px; }
        .graph-title { font-size: 11px; color: #8b949e; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 1px; font-weight: 600; }
        .status-badge { display: inline-block; padding: 4px 10px; border-radius: 12px; font-size: 12px; font-weight: 600; background: #238636; color: #ffffff; margin-left: 10px; }
        .status-badge.down { background: #da3633; }
        img { display: block; max-width: 100%; height: auto; }
        .meta-text { color: #8b949e; font-size: 14px; margin-bottom: 15px; }
    </style>
</head>
<body>
    <div class="container">
`)

	mdBuf.WriteString("# Local Environment Dashboard\n\n")

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

	mdBuf.WriteString(fmt.Sprintf("**Host Connectivity:** %s (%d historical outages recorded)\n\n", statusText, outages))
	htmlBuf.WriteString(fmt.Sprintf(`<h1>Environment Dashboard</h1><p class="meta-text"><strong>Host Connectivity:</strong> %s (%d historical outages recorded)</p>`, statusText, outages))

	mdBuf.WriteString("## Active Services & Codebases\n\n")
	htmlBuf.WriteString("<h2>Active Services & Codebases</h2>")

	if len(metricsData.Repos) == 0 {
		mdBuf.WriteString("_No active local services detected._\n")
		htmlBuf.WriteString("<p><em>No active local services detected.</em></p>")
	}

	for _, repo := range metricsData.Repos {
		repoTitle := fmt.Sprintf("### 🐳 `%s`", repo.Name)
		htmlTitle := fmt.Sprintf(`<h3>🐳 %s</h3>`, repo.Name)
		if repo.RemoteURL != "" {
			repoTitle = fmt.Sprintf("### 🐳 [%s](%s)", repo.Name, repo.RemoteURL)
			htmlTitle = fmt.Sprintf(`<h3>🐳 <a href="%s" style="color: #58a6ff; text-decoration: none;">%s</a></h3>`, repo.RemoteURL, repo.Name)
		}
		
		mdBuf.WriteString(repoTitle + "\n")
		htmlBuf.WriteString(fmt.Sprintf(`<div class="repo-card">%s`, htmlTitle))

		dirtyStr := ""
		if repo.Dirty {
			dirtyStr = " [dirty]"
		}

		gitStats := fmt.Sprintf("branch <code>%s</code>%s &middot; %d total commits &middot; <strong>%d PRs landed in last 7 days</strong>", repo.Branch, dirtyStr, repo.Commits, repo.RecentPRs)
		mdGitStats := fmt.Sprintf("branch `%s`%s, %d total commits, %d PRs landed in last 7 days", repo.Branch, dirtyStr, repo.Commits, repo.RecentPRs)

		if repo.OpenPRs >= 0 {
			gitStats += fmt.Sprintf(" &middot; %d open PRs", repo.OpenPRs)
			mdGitStats += fmt.Sprintf(", %d open PRs", repo.OpenPRs)
		}
		if repo.Unpushed > 0 {
			gitStats += fmt.Sprintf(` &middot; <span style="color: #e3b341">%d unpushed commits</span>`, repo.Unpushed)
			mdGitStats += fmt.Sprintf(", %d unpushed commits", repo.Unpushed)
		}

		mdBuf.WriteString(fmt.Sprintf("* **Git:** %s\n", mdGitStats))
		htmlBuf.WriteString(fmt.Sprintf(`<p class="meta-text"><strong>Git:</strong> %s</p>`, gitStats))

		mdBuf.WriteString("* **Services:**\n")
		htmlBuf.WriteString(`<ul class="service-list">`)

		for _, s := range repo.Services {
			mdBuf.WriteString(fmt.Sprintf("  - `%s` (`%s`) - **%s**\n", s.Name, s.Image, s.Status))

			badgeClass := "status-badge"
			if strings.Contains(strings.ToLower(s.Status), "exited") || strings.Contains(strings.ToLower(s.Status), "down") {
				badgeClass += " down"
			}
			htmlBuf.WriteString(fmt.Sprintf(`<li class="service-item"><strong>%s</strong> <code style="margin-left: 8px; color: #8b949e;">%s</code> <span class="%s">%s</span>`, s.Name, s.Image, badgeClass, s.Status))

			// Generate graphs
			hist := store.GetServiceHistory(s.Name)
			if len(hist) > 0 {
				safeName := strings.ReplaceAll(s.Name, "/", "_")
				safeName = strings.ReplaceAll(safeName, ":", "_")
				uptimeFile := fmt.Sprintf("%s_uptime.svg", safeName)
				churnFile := fmt.Sprintf("%s_churn.svg", safeName)

				uptimeSVG := makeUptimeSVG(hist)
				churnSVG := makeChurnSVG(repo.CommitTimestamps)

				os.WriteFile(filepath.Join(reportDir, uptimeFile), []byte(uptimeSVG), 0644)
				os.WriteFile(filepath.Join(reportDir, churnFile), []byte(churnSVG), 0644)

				mdBuf.WriteString(fmt.Sprintf("    * Uptime:<br>![Uptime](%s)\n", uptimeFile))
				mdBuf.WriteString(fmt.Sprintf("    * Churn:<br>![Churn](%s)\n", churnFile))

				htmlBuf.WriteString(fmt.Sprintf(`
                <div class="graphs">
                    <div class="graph-box"><div class="graph-title">Uptime (Last 7 days)</div><img src="%s" alt="Uptime"></div>
                    <div class="graph-box"><div class="graph-title">Churn (Last 7 days)</div><img src="%s" alt="Churn"></div>
                </div>`, uptimeFile, churnFile))
			}
			htmlBuf.WriteString("</li>")
		}
		mdBuf.WriteString("\n")
		htmlBuf.WriteString("</ul></div>")
	}

	htmlBuf.WriteString("</div></body></html>")

	// Write Markdown
	os.WriteFile(filepath.Join(reportDir, "uptime.md"), mdBuf.Bytes(), 0644)

	// Write HTML
	os.WriteFile(filepath.Join(reportDir, "index.html"), htmlBuf.Bytes(), 0644)

	// Write JSON
	jsonData, _ := json.MarshalIndent(metricsData, "", "  ")
	os.WriteFile(filepath.Join(reportDir, "data.json"), jsonData, 0644)

	// Write XML
	var xmlDash XMLDashboard
	for _, r := range metricsData.Repos {
		xr := XMLRepo{
			Name:      r.Name,
			RemoteURL: r.RemoteURL,
			Branch:    r.Branch,
			Dirty:     r.Dirty,
			Commits:   r.Commits,
			RecentPRs: r.RecentPRs,
			Unpushed:  r.Unpushed,
			OpenPRs:   r.OpenPRs,
		}
		for _, s := range r.Services {
			xr.Services = append(xr.Services, XMLService{
				Name:   s.Name,
				Image:  s.Image,
				Status: s.Status,
			})
		}
		xmlDash.Repos = append(xmlDash.Repos, xr)
	}
	xmlData, _ := xml.MarshalIndent(xmlDash, "", "  ")
	xmlData = append([]byte(xml.Header), xmlData...)
	os.WriteFile(filepath.Join(reportDir, "data.xml"), xmlData, 0644)

	return nil
}

func generateSVGGrid(colors []string) string {
	var buf bytes.Buffer
	buf.WriteString(`<svg width="312" height="91" xmlns="http://www.w3.org/2000/svg">` + "\n")

	// 24 cols x 7 rows
	for i := 0; i < 168; i++ {
		col := i / 7
		row := i % 7
		x := col * 13
		y := row * 13

		color := "#ebedf0"
		if i < len(colors) {
			color = colors[i]
		}

		buf.WriteString(fmt.Sprintf(`  <rect x="%d" y="%d" width="10" height="10" fill="%s" rx="2" ry="2"/>`+"\n", x, y, color))
	}
	buf.WriteString("</svg>")
	return buf.String()
}

func makeUptimeSVG(history []state.ServiceSnapshot) string {
	padded := make([]state.ServiceSnapshot, 672)
	startIdx := 672 - len(history)
	for i, h := range history {
		padded[startIdx+i] = h
	}

	colors := make([]string, 168)
	for i := 0; i < 168; i++ {
		upCount := 0
		hasData := false
		for j := 0; j < 4; j++ {
			p := padded[i*4+j]
			if !p.Timestamp.IsZero() {
				hasData = true
				if p.Up {
					upCount++
				}
			}
		}

		if !hasData {
			colors[i] = "#ebedf0"
		} else if upCount > 0 {
			colors[i] = "#30a14e" // green
		} else {
			colors[i] = "#f85149" // red
		}
	}

	return generateSVGGrid(colors)
}

func makeChurnSVG(timestamps []time.Time) string {
	colors := make([]string, 168)
	
	now := time.Now()
	buckets := make([]int, 168)
	
	for _, t := range timestamps {
		hoursAgo := int(now.Sub(t).Hours())
		if hoursAgo >= 0 && hoursAgo < 168 {
			idx := 167 - hoursAgo
			buckets[idx]++
		}
	}
	
	for i := 0; i < 168; i++ {
		totalChurn := buckets[i]
		
		c := "#ebedf0"
		if totalChurn == 1 || totalChurn == 2 {
			c = "#9be9a8"
		} else if totalChurn >= 3 && totalChurn <= 5 {
			c = "#40c463"
		} else if totalChurn >= 6 && totalChurn <= 10 {
			c = "#30a14e"
		} else if totalChurn > 10 {
			c = "#216e39"
		}
		colors[i] = c
	}

	return generateSVGGrid(colors)
}

// CommitDashboard commits the REPORT dir locally without pushing.
const daemonCommitMsg = "chore: update uptime dashboard and exported formats"
const daemonAuthorEmail = "transparent@local"

func CommitDashboard(ctx context.Context, repoPath string) error {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("failed to access worktree: %w", err)
	}

	_, err = w.Add("REPORT")
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

	// If HEAD is a previous daemon commit, soft-reset to its parent so we
	// effectively amend it — keeping REPORT/ as a single living tip commit.
	head, err := r.Head()
	if err == nil {
		headCommit, err := r.CommitObject(head.Hash())
		if err == nil &&
			headCommit.Author.Email == daemonAuthorEmail &&
			headCommit.Message == daemonCommitMsg &&
			len(headCommit.ParentHashes) == 1 {

			err = w.Reset(&git.ResetOptions{
				Commit: headCommit.ParentHashes[0],
				Mode:   git.SoftReset,
			})
			if err != nil {
				return fmt.Errorf("soft reset failed: %w", err)
			}

			// Re-stage REPORT after the reset
			if _, err = w.Add("REPORT"); err != nil {
				return fmt.Errorf("git add after reset failed: %w", err)
			}
		}
	}

	_, err = w.Commit(daemonCommitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Transparent Daemon",
			Email: daemonAuthorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}
