# transparent

A self-hosted engineering observability daemon. It runs alongside your Docker fleet, reads your local git repositories, and publishes a live `REPORT/` directory — containing Markdown, HTML, JSON, and XML — that gives you a GitHub-style view of service uptime and code churn for every project you have running.

Think of it as the lazy engineer's Grafana: no metrics server, no dashboards to configure, no agents to install. Just a single container that commits its findings to your repo on a schedule.

## What it does

Every 15 minutes, `transparent` polls your environment and produces:

- **`REPORT/uptime.md`** — GitHub Flavored Markdown dashboard, renderable directly on GitHub
- **`REPORT/index.html`** — Standalone HTML dashboard, viewable in any browser without a Markdown viewer
- **`REPORT/data.json`** — Raw metrics payload
- **`REPORT/data.xml`** — Structured XML for downstream integrations
- **`REPORT/{service}_uptime.svg`** — 7-day uptime contribution grid per service
- **`REPORT/{service}_churn.svg`** — 7-day code churn contribution grid per service

The daemon commits these outputs to your repo as a single amended tip commit (`transparent@local`) so the dashboard stays up to date without polluting your git history.

## Metrics collected

For each repository under `~/work` that has at least one running Docker container:

| Metric | Source |
|--------|--------|
| Current branch | `go-git` |
| Total commit count | `go-git` log |
| Dirty working tree | `go-git` status |
| PRs landed (last 7 days) | `go-git` merge commits |
| Open pull requests | GitHub API |
| Unpushed commits | Local HEAD vs `refs/remotes/origin/<branch>` |
| Per-service uptime | `docker ps` status string |
| Code churn history | Real commit timestamps binned per hour |

Service → repository mapping is done by container name prefix (e.g. `odysseus-odysseus-1` maps to `~/work/odysseus`). Third-party images running inside a project's compose stack are attributed to that project.

## Graphs

The SVG contribution grids use GitHub's exact color scale:

| Color | Meaning |
|-------|---------|
| `#ebedf0` | No data / zero commits |
| `#9be9a8` | 1–2 commits in window |
| `#40c463` | 3–5 commits |
| `#30a14e` | 6–10 commits / service up |
| `#216e39` | >10 commits |
| `#f85149` | Service down |

Each grid is 24 columns × 7 rows = 168 hourly buckets covering a rolling 7-day window. State history is persisted across restarts in `data.json`.

## Running

```bash
docker compose up -d
```

The container mounts:

| Mount | Purpose |
|-------|---------|
| `.:/data` | Repo root — where `REPORT/` is written and committed |
| `~/work:/work:ro` | Scanned for git repos with active containers |
| `/var/run/docker.sock:/var/run/docker.sock:ro` | Container inspection |
| `~/.ssh:/root/.ssh:ro` | Git remote access |

Pass `--immediate` (already set in `docker-compose.yml`) to trigger a full evaluation on startup rather than waiting for the first 15-minute tick.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-repo` | required | Path to the repo `transparent` commits into |
| `-workdir` | `/work` | Root directory scanned for project repos |
| `-poll` | `15m` | How often to collect metrics and update state |
| `-commit` | `1h` | How often to generate and commit the dashboard |
| `-immediate` | `false` | Run an evaluation immediately on startup |

## Architecture

```
cmd/transparent/main.go   — event loop, flag parsing, signal handling
pkg/metrics/              — docker + go-git data collection
pkg/state/                — time-series snapshot store (persisted to data.json)
pkg/reporter/             — SVG generation, multi-format rendering, git commit
```

The commit loop is intentionally decoupled from the poll loop. State snapshots are recorded every 15 minutes; the dashboard is regenerated and committed on a separate 1-hour cadence (configurable). If the previous HEAD commit belongs to the daemon, it is soft-reset and replaced rather than appended, keeping a clean one-commit-per-dashboard model.

## Requirements

- Docker with socket access
- Git repos under `~/work` with an `origin` remote (GitHub URLs resolved automatically from both `git@github.com:` and `https://` forms)
- No GitHub token required for public repos; open PR counts will return `-1` for private repos without one

## Output example

```
### 🐳 odysseus
* **Git:** branch `dev` [dirty], 1150 total commits, 9 PRs landed in last 7 days, 30 open PRs
* **Services:**
  - `odysseus-odysseus-1` (odysseus-odysseus) — Up 3 hours
    * Uptime: ![Uptime](odysseus-odysseus-1_uptime.svg)
    * Churn:  ![Churn](odysseus-odysseus-1_churn.svg)
```
