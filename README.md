# transparent

`transparent` is a localized observability daemon. It runs alongside the Docker fleet, inspects local git repositories, and publishes an immutable `REPORT/` directory (Markdown, HTML, JSON, XML). This produces a deterministic view of service uptime and codebase churn without reliance on external TSDBs.

It eliminates the overhead of managing distributed metric agents and dashboards by collapsing observability into a single, scheduled batch process that emits statically generated artifacts.

## Architecture

```text
cmd/transparent/main.go   — Control loop, metrics endpoint (:8080/metrics)
pkg/metrics/              — State collection (Docker Engine API, go-git)
pkg/state/                — Local time-series persistence (data.json)
pkg/reporter/             — Artifact generation (Markdown, SVG grids)
pkg/telemetry/            — Zero-dependency Prometheus exposition
```

The daemon decouples the state collection loop (15-minute tick) from the dashboard emission loop (1-hour tick). State snapshots are persisted locally. When emitting the dashboard, the daemon performs a soft-reset on the previous `transparent@local` commit to amend the repository HEAD, ensuring git history remains strictly unpolluted.

## Artifacts

Every emission cycle produces the following artifacts within the host repository:

- **`REPORT/uptime.md`** — Core Markdown dashboard.
- **`REPORT/index.html`** — Standalone HTML index.
- **`REPORT/data.json`** — Raw state matrix.
- **`REPORT/data.xml`** — Structured interchange format.
- **`REPORT/{service}_uptime.svg`** — 168-hour rolling uptime grid.
- **`REPORT/{service}_churn.svg`** — 168-hour rolling commit grid.

## Invariants & Metrics

The daemon scans `~/work` for repositories bound to active Docker containers. Service-to-repository attribution is determined by container name prefix constraints.

| Metric | Origin |
|--------|--------|
| Git Branch / Dirty State | `go-git` (`HEAD` ref mapping) |
| Total Commit Count | `go-git` |
| PRs Landed (7d) | `go-git` (Merge commit traversal) |
| Open Pull Requests | GitHub REST API |
| Unpushed Commits | `HEAD` vs `refs/remotes/origin/<branch>` |
| Service Uptime | Docker Engine socket (`docker ps`) |
| Code Churn Density | Commit timestamp binning (1h buckets) |

## Fleet Integration (MCP)

`transparent` natively implements the Model Context Protocol (MCP). 

Agent fleets can ingest the skill `transparent_get_fleet_status` dynamically. The daemon exposes `GET /dashboard` and `GET /metrics` on `:8080`, allowing autonomous orchestrators to consume fleet telemetry and dashboard state without parsing local artifacts.

## Execution

```bash
docker compose up -d
```

### Mounts

| Mount | Purpose |
|-------|---------|
| `.:/data` | Output directory for `REPORT/` emission. |
| `~/work:/work:ro` | Read-only scan target for active repositories. |
| `/var/run/docker.sock:/var/run/docker.sock:ro` | Read-only Docker socket for container inspection. |
| `~/.ssh:/root/.ssh:ro` | Read-only SSH keys for git remote resolution. |

### Configuration

| Flag | Default | Constraint |
|------|---------|-------------|
| `-repo` | `.` | Target repository path for emission. |
| `-workdir` | `/work` | Root directory for project resolution. |
| `-poll` | `15m` | Telemetry evaluation interval. |
| `-commit` | `1h` | Artifact generation and commit interval. |
| `-immediate` | `false` | Force execution before initial tick. |

## Dependencies

- Docker socket bound to host.
- Standard git repository structures located in `~/work`.
- Public repositories require no authentication. Private repositories require standard credential payloads.
