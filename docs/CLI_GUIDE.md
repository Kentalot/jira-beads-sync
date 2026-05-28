# CLI Guide

Complete reference for using jira-beads-sync from the command line.

## Table of Contents

- [Overview](#overview)
- [Commands](#commands)
  - [configure](#configure)
  - [quickstart](#quickstart)
  - [sync](#sync)
  - [convert](#convert)
  - [version](#version)
  - [help](#help)
- [Configuration](#configuration)
- [Examples](#examples)

## Overview

The jira-beads-sync CLI provides commands for importing Jira issues, syncing changes, and converting exported data. All commands follow this pattern:

```bash
jira-beads-sync <command> [options] [arguments]
```

## Commands

### configure

Interactive configuration wizard for setting up Jira API credentials.

**Usage:**
```bash
jira-beads-sync configure
```

**What it does:**
- Prompts for Jira base URL (e.g., `https://yourcompany.atlassian.net`)
- Prompts for Jira username (your email address)
- Prompts for API token
- Saves configuration to `~/.config/jira-beads-sync/config.yml`

**Example:**
```bash
$ jira-beads-sync configure
Enter Jira Base URL (e.g., https://yourcompany.atlassian.net): https://acme.atlassian.net
Enter Jira Username (email): user@example.com
Enter Jira API Token: **********************
Configuration saved to /home/user/.config/jira-beads-sync/config.yml
```

**Getting an API Token:**
Visit https://id.atlassian.com/manage-profile/security/api-tokens to create a new token.

### quickstart

Fetch issues directly from Jira API and sync them to beads format. This is the recommended way to import issues as it supports bidirectional sync.

**Usage:**
```bash
jira-beads-sync quickstart <jira-url-or-key>
```

**Arguments:**
- `<jira-url-or-key>`: Either a full Jira URL or just the issue key

**Options:**
- Uses configuration from `~/.config/jira-beads-sync/config.yml`
- Can be overridden with environment variables (see [Configuration](#configuration))

**What it does:**
1. Fetches the specified issue from Jira REST API v2
2. Recursively walks the dependency graph:
   - All subtasks
   - All linked issues (blocks, depends on, relates to)
   - Parent issues (excluding epics, which become beads epics)
   - Transitive dependencies
3. Prevents duplicates using visited tracking
4. Converts all issues to beads format
5. Creates YAML files in `.beads/issues/` directory

**Examples:**

Import using issue key (uses base URL from config):
```bash
jira-beads-sync quickstart PROJ-123
```

Import using full URL:
```bash
jira-beads-sync quickstart https://acme.atlassian.net/browse/PROJ-123
```

Import an epic (fetches all stories and subtasks):
```bash
jira-beads-sync quickstart PROJ-100
```

**Output:**
```
Fetching PROJ-123...
Fetched issue: PROJ-123 - Implement authentication system
Walking dependencies...
  Fetching subtask: PROJ-124 - Add OAuth2 support
  Fetching subtask: PROJ-125 - Update user model
  Fetching linked issue: PROJ-110 - Design auth flow (blocks)
✓ Fetched 4 issue(s)
Converting to beads format...
✓ Conversion complete!
Issues created in .beads/issues/
```

### sync

Sync beads state changes back to Jira via the API.

**Usage:**
```bash
jira-beads-sync sync <issue-keys...>
```

**Arguments:**
- `<issue-keys...>`: **Required.** One or more Jira keys to sync (e.g., `PROJ-123` or `PROJ-123 PROJ-456`). Only issues in `.beads/issues.jsonl` whose `metadata.jiraKey` (or `external_ref` `jira-KEY`) matches a listed key are pushed. This prevents stale sibling issues imported via `quickstart` from being synced accidentally.

**What it does:**
1. Loads `.beads/issues.jsonl` from the current directory.
2. For each mapped issue selected for sync, fetches current Jira state and compares beads fields.
3. **Native beads comments** (`bd comment` / `bd comments add`): loads comments from the `comments` array on each issues.jsonl line (when present) or via `bd comments <id> --json` in the repo. Eligible comments are posted as Jira issue comments and their beads comment IDs are recorded in `metadata.jiraPostedCommentIds` so they are **never posted twice**. Default policy (`tagged`): only comments whose text contains `#jira` are pushed; the tag is stripped from the Jira body. Set `jira.sync_beads_comments_policy: all` (or `JIRA_SYNC_BEADS_COMMENTS=all`) to push every new beads comment; `off` disables this path. After posting, **`issues.jsonl` is rewritten immediately** so a disk failure cannot leave unrecorded IDs that would duplicate on retry.
4. Updates Jira via REST API (summary, description when safe, priority, assignee, status transitions).
5. Comment API: tries `POST /rest/api/2/issue/{key}/comment` with a plain string body; if that fails (common on some Jira Cloud setups), retries with `POST /rest/api/3/issue/{key}/comment` using minimal Atlassian Document Format (ADF)

**Native beads JSONL:** lines may include extra top-level keys (`_type`, `created_at`, `close_reason`, etc.) and non-string values inside `metadata`. Sync **preserves** those on save; only string entries in `metadata` are mirrored into the fields this tool reads.

**Sync metadata (on the beads issue):**
- `jiraPostedCommentIds` — comma-separated beads comment IDs already posted to Jira (managed by sync; do not edit unless recovering from a partial run).

**Beads comments → Jira (opt-in):**
- Add `#jira` anywhere in the beads comment text (e.g. `bd comment bd-abc "Fix verified #jira"`).
- On sync, the comment is posted to the mapped Jira issue; `#jira` is removed from the Jira text; author is prefixed as `[Author] …` when present.
- Config `jira.sync_beads_comments_policy` (or env `JIRA_SYNC_BEADS_COMMENTS`): `tagged` (default), `all`, or `off`.

**Description behavior:**
- If Jira’s description is **ADF / rich text** that we cannot parse to plain text, sync **never overwrites** the Jira description (avoids wiping rich descriptions when beads only has a short plain-text delta). A warning is printed for that issue.
- Config `jira.sync_description_policy` (or env `JIRA_SYNC_DESCRIPTION_POLICY`): `replace` (default) pushes description when it differs and the remote is plain text; `skip` never pushes description from beads to Jira.

**Examples:**

```bash
jira-beads-sync sync PROJ-123
jira-beads-sync sync PROJ-123 PROJ-456
```

**Status Mapping (beads → Jira):**
- `open` → "To Do" or "Open"
- `in_progress` → "In Progress"
- `blocked` → "Blocked"
- `closed` → "Done" or "Closed"

**Priority Mapping (beads → Jira):**
- `p0` (critical) → "Blocker" or "Critical"
- `p1` → "Highest"
- `p2` → "High" or "Medium"
- `p3` → "Low"
- `p4` → "Lowest"

**Note:** Sync mode is under active development. Some features may be limited in the current release.

**Limitations (current release):**
- Sync applies to **`.beads/issues.jsonl` only** (issues imported via `quickstart` / `fetch-by-label` / `fetch-jql`). Epics in `.beads/epics.jsonl` are not pushed back to Jira yet.
- Each issue must map to Jira using **`metadata.jiraKey`** (set by this tool on import) **or** **`external_ref`** in the form **`jira-PROJ-123`** (some native beads databases).
- Status changes use Jira **workflow transitions**. The tool picks a transition whose destination status maps to your beads status (`open`, `in_progress`, `blocked`, `closed`). If no such transition exists from the issue’s current state, sync reports an error for that issue.
- Assignee updates resolve the beads assignee via **`GET /rest/api/3/user/search`** on Jira Cloud (`accountId`), with fallback to **`GET /rest/api/2/user/search`** and **`/user/assignable/search`** on Jira Server/Data Center (`name`). Prefer the user’s **email** in beads when possible.

### convert

One-way conversion of previously exported Jira JSON files to beads format. Use this for archived projects or when API access is not available.

**Usage:**
```bash
jira-beads-sync convert <json-file>
```

**Arguments:**
- `<json-file>`: Path to a Jira export JSON file

**What it does:**
1. Reads the Jira JSON export file
2. Parses issue data, relationships, and metadata
3. Converts to beads protobuf format
4. Renders to YAML files in `.beads/issues/`

**Examples:**

Convert a Jira export:
```bash
jira-beads-sync convert jira-export.json
```

Convert with relative path:
```bash
jira-beads-sync convert ./exports/sprint-42.json
```

**Limitations:**
- **One-way only**: Cannot sync changes back to Jira
- **No API required**: Works offline, doesn't need credentials
- **Static data**: Uses snapshot from export time

**When to use convert vs quickstart:**
- Use **convert** for: Archived projects, offline processing, no API access
- Use **quickstart** for: Active projects, bidirectional sync, current data

### version

Display the version of jira-beads-sync.

**Usage:**
```bash
jira-beads-sync version
```

**Output:**
```
jira-beads-sync version 0.1.0
```

### help

Display help information for commands.

**Usage:**
```bash
jira-beads-sync help [command]
```

**Examples:**

General help:
```bash
jira-beads-sync help
```

Command-specific help:
```bash
jira-beads-sync help quickstart
jira-beads-sync help sync
```

## Configuration

jira-beads-sync supports multiple configuration methods with the following precedence (highest to lowest):

### 1. Environment Variables (Highest Priority)

Set these in your shell or CI/CD environment:

```bash
export JIRA_BASE_URL=https://acme.atlassian.net
export JIRA_USERNAME=user@example.com
export JIRA_API_TOKEN=your-api-token-here
# Optional: never push beads description to Jira (summary/status/etc. still sync)
# export JIRA_SYNC_DESCRIPTION_POLICY=skip
```

Then run commands without additional setup:
```bash
jira-beads-sync quickstart PROJ-123
```

### 2. Config File

Located at `~/.config/jira-beads-sync/config.yml`:

```yaml
jira:
  base_url: https://acme.atlassian.net
  username: user@example.com
  api_token: your-api-token-here
  # Optional: replace (default) or skip — whether to push beads description to Jira when safe
  # sync_description_policy: skip
```

Create this file manually or use `jira-beads-sync configure`.

### 3. Interactive Configuration

If no configuration is found, you'll be prompted:

```bash
$ jira-beads-sync quickstart PROJ-123
No configuration found. Please run 'jira-beads-sync configure' first.
```

## Examples

### First-Time Setup

```bash
# Configure credentials
jira-beads-sync configure

# Import your first issue
jira-beads-sync quickstart PROJ-123

# View imported issues (requires beads)
bd list
bd show proj-123
```

### Import an Epic with All Stories

```bash
# Fetch epic and all related issues
jira-beads-sync quickstart AUTH-100

# The tool automatically fetches:
# - All stories in the epic
# - All subtasks of those stories
# - All linked dependencies
```

### Work Across Multiple Projects

```bash
# Import from different projects
jira-beads-sync quickstart PROJ-123
jira-beads-sync quickstart TEAM-456
jira-beads-sync quickstart INFRA-789

# All issues are stored in .beads/issues/
# Cross-project dependencies are preserved
```

### Use in CI/CD

```bash
# Set credentials via environment variables
export JIRA_BASE_URL=$JIRA_URL
export JIRA_USERNAME=$JIRA_USER
export JIRA_API_TOKEN=$JIRA_TOKEN

# Import issues in pipeline
jira-beads-sync quickstart $EPIC_KEY

# Process or analyze issues
bd list --status open
```

### Convert Legacy Exports

```bash
# Convert old Jira exports
jira-beads-sync convert archive/2024-q1-export.json
jira-beads-sync convert archive/2024-q2-export.json

# Issues are converted but cannot be synced back
```

### Sync Workflow

```bash
# 1. Import from Jira
jira-beads-sync quickstart PROJ-123

# 2. Work locally with beads
bd update proj-123 --status in_progress
bd update proj-124 --assignee alice
bd close proj-125

# 3. Sync changes back to Jira (list every key you changed)
jira-beads-sync sync PROJ-123 PROJ-124 PROJ-125
```

### Scripting and Automation

```bash
#!/bin/bash
# Import issues from a list

ISSUES=(
  "PROJ-123"
  "PROJ-456"
  "PROJ-789"
)

for issue in "${ISSUES[@]}"; do
  echo "Importing $issue..."
  jira-beads-sync quickstart "$issue"
done

echo "✓ All issues imported"
bd list
```

## Troubleshooting

### Authentication Errors

**Problem:** `Authentication failed: 401 Unauthorized`

**Solutions:**
- Verify your API token is correct and hasn't expired
- Check that your username (email) is correct
- Ensure you have access to the Jira project
- Create a new API token at https://id.atlassian.com/manage-profile/security/api-tokens

### Configuration Not Found

**Problem:** `No configuration found`

**Solutions:**
- Run `jira-beads-sync configure` to set up credentials
- Or set environment variables: `JIRA_BASE_URL`, `JIRA_USERNAME`, `JIRA_API_TOKEN`
- Or create `~/.config/jira-beads-sync/config.yml` manually

### Issue Not Found

**Problem:** `Issue PROJ-123 not found: 404`

**Solutions:**
- Verify the issue key is correct (case-sensitive)
- Check that you have read permissions on the issue
- Ensure the issue exists in Jira
- Verify the base URL points to the correct Jira instance

### Network Errors

**Problem:** `Failed to fetch issue: connection timeout`

**Solutions:**
- Check your internet connection
- Verify the Jira base URL is correct and accessible
- Check if your organization uses a proxy (may need additional configuration)
- Verify Jira is not experiencing an outage

### Dependency Loops

**Problem:** Tool seems stuck fetching issues

**Solution:**
The tool includes circular dependency detection and visited tracking. If you experience this:
- The dependency graph may be very large (check Jira web UI)
- Press Ctrl+C to cancel and try a specific issue instead of an epic

## Advanced Usage

### Custom Output Directory

```bash
# Change directory before running
cd /path/to/project
jira-beads-sync quickstart PROJ-123
# Creates .beads/issues/ in current directory
```

### Combining with beads Commands

```bash
# Import and immediately view
jira-beads-sync quickstart PROJ-123 && bd show proj-123

# Import multiple issues and list them
jira-beads-sync quickstart PROJ-123
jira-beads-sync quickstart PROJ-456
bd list --format json
```

### Docker Usage

```bash
# Run in Docker container
docker run --rm \
  -v $(pwd):/data \
  -e JIRA_BASE_URL \
  -e JIRA_USERNAME \
  -e JIRA_API_TOKEN \
  ghcr.io/conallob/jira-beads-sync:latest \
  quickstart PROJ-123
```

## Next Steps

- Learn about the [Claude Code Plugin](PLUGIN_GUIDE.md)
- See [Real-World Examples](EXAMPLES.md)
- Read [Architecture Details](../CLAUDE.md) for development

## Getting Help

- **Documentation**: [Main README](../README.md)
- **Issues**: https://github.com/Kentalot/jira-beads-sync/issues
- **Examples**: [EXAMPLES.md](EXAMPLES.md)
