# bs-pipeline

Check the GitHub Actions CI pipeline status after a push.

## Usage

`/bs-pipeline` — check the latest workflow run on the current branch
`/bs-pipeline <run-id>` — check a specific workflow run

## Steps

1. Run: `gh run list --branch "$(git branch --show-current)" --limit 5 --json databaseId,status,conclusion,name,headBranch,createdAt`
2. Display a summary table of recent runs (status, conclusion, name, branch, time)
3. If the most recent run is `in_progress` or `queued`:
   - Wait 15 seconds, then re-check with `gh run view <id> --json status,conclusion,jobs`
   - Repeat up to 20 times (5 minutes max)
4. Once the most recent run completes, run: `gh run view <id> --json jobs --jq '.jobs[] | select(.conclusion != "success") | {name, conclusion, steps: [.steps[] | select(.conclusion != "success" and .conclusion != "skipped")]}'`
5. If all jobs passed: report success with job names
6. If any jobs failed: show the failed job names and failing step names, then run `gh run view <id> --log-failed | tail -50` to show relevant failure output
