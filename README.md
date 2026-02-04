# DORA Metrics Calculator

A CLI tool to measure DORA Four Keys metrics using the GitHub API.

## What is DORA Four Keys?

DORA Four Keys are the four key metrics defined by DevOps Research and Assessment (DORA) to measure software delivery performance.

| Metric | Description | Definition in this tool |
|--------|-------------|------------------------|
| **Deployment Frequency** | How often deploys to production | Number of merges to main branch |
| **Lead Time for Changes** | Time from commit to production deploy | Time from first commit to PR merge |
| **Change Failure Rate** | Percentage of deployments causing failures | Ratio of hotfix/bugfix PRs + bug labels + reverts |
| **Time to Restore Service** | Time to recover from failures | *Not measured in this tool* |

### Additional Metrics

| Metric | Description |
|--------|-------------|
| **Time to First Review** | Time from PR creation to first review |

## Setup

### 1. Build

```bash
go build -o dora-metrics .
```

### 2. Configure Environment Variables

```bash
cp .env.example .env
vim .env
```

### 3. Get GitHub Token

Create a token at [GitHub Settings > Developer settings > Personal access tokens](https://github.com/settings/tokens) with the following permissions:

- `repo` (for private repositories)
- `public_repo` (for public repositories only)

## Configuration

### .env File

```bash
# GitHub API Token (required)
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx

# GitHub Organization or User (required)
GITHUB_OWNER=your-org

# Repository names (comma-separated, required)
GITHUB_REPOS=repo1,repo2,repo3

# Team members (comma-separated, optional)
# If not specified, all contributors are included
GITHUB_MEMBERS=user1,user2,user3

# Analysis period (YYYY-MM-DD format, required)
DORA_FROM=2025-01-01
DORA_TO=2025-01-31
```

## Usage

### Basic

```bash
# If .env is configured, just specify the period
./dora-metrics --from 2025-01-01 --to 2025-01-31
```

### Specify All Options via Command Line

```bash
./dora-metrics \
  --owner your-org \
  --repos repo1,repo2,repo3 \
  --members user1,user2 \
  --from 2025-01-01 \
  --to 2025-01-31 \
  --token ghp_xxxx
```

### Single Repository

```bash
./dora-metrics \
  --owner your-org \
  --repos single-repo \
  --from 2025-01-01 \
  --to 2025-01-31
```

### All Contributors

```bash
# Omit --members to include all contributors
./dora-metrics \
  --owner your-org \
  --repos repo1 \
  --from 2025-01-01 \
  --to 2025-01-31
```

## Options

| Option | Environment Variable | Description | Required |
|--------|---------------------|-------------|----------|
| `--owner` | `GITHUB_OWNER` | GitHub Organization or User | Yes |
| `--repos` | `GITHUB_REPOS` | Repository names (comma-separated) | Yes |
| `--from` | `DORA_FROM` | Start date (YYYY-MM-DD) | Yes |
| `--to` | `DORA_TO` | End date (YYYY-MM-DD) | Yes |
| `--members` | `GITHUB_MEMBERS` | Filter by members (comma-separated) | No |
| `--token` | `GITHUB_TOKEN` | GitHub API Token | Yes |

## Example Output

```
🚀 DORA Metrics Calculator
   Organization: your-org
   Repositories: [repo1 repo2]
   Members: [user1 user2]
   Period: 2025-01-01 ~ 2025-01-31

📊 Analyzing your-org/repo1...
   Found 23 merged PRs

╔══════════════════════════════════════════════════════════════════╗
║  DORA Metrics: repo1
║  Period: 2025-01-01 ~ 2025-01-31
╠══════════════════════════════════════════════════════════════════╣
║
║  📦 Deployment Frequency
║     Total Deployments: 23
║     Frequency: 0.74 deploys/day (5.2 deploys/week)
║
║  ⏱️  Lead Time for Changes (first commit → merge)
║     Average: 2.3 days
║     Median:  1.1 days
║
║  🔥 Change Failure Rate
║     Failure PRs + Reverts: 2
║     Rate: 8.7%
║
║  👀 Time to First Review (PR created → first review)
║     Average: 5.2 hours
║     Median:  2.8 hours
║
╠══════════════════════════════════════════════════════════════════╣
║  👥 Per-Member Breakdown
╠══════════════════════════════════════════════════════════════════╣
║
║  @user1
║     PRs Merged: 15
║     Lead Time (avg/median): 1.8 days / 0.9 days
║     Time to First Review (avg/median): 4.1 hours / 2.3 hours
║     Failure PRs: 1
║
║  @user2
║     PRs Merged: 8
║     Lead Time (avg/median): 3.2 days / 1.5 days
║     Time to First Review (avg/median): 7.1 hours / 3.5 hours
║     Failure PRs: 1
║
╚══════════════════════════════════════════════════════════════════╝
```

When multiple repositories are specified, a Combined Summary is also displayed at the end.

## Change Failure Criteria

PRs matching any of the following are counted as failure PRs:

1. **Branch name**: Contains `hotfix` or `bugfix`
2. **Labels**: Contains `bug`, `hotfix`, or `bugfix`
3. **Revert commits**: Commits on main branch starting with `Revert`

## Limitations

- Subject to GitHub API rate limits (5,000 requests/hour for authenticated users)
- API calls may take time for repositories with many PRs
- Time to Restore Service (MTTR) is not measured in this tool

## License

MIT
