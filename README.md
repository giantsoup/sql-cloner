# dbgold

`dbgold` is a Go TUI for creating and restoring local MySQL "golden" snapshots with MySQL Shell.

It is designed as a friendlier replacement for the common `mysqldump` plus "replay a SQL script" local workflow, while usually being much faster for larger development databases.

## Features

- Full-screen keyboard-driven TUI built with Bubble Tea, Bubbles, Huh, and Lip Gloss
- First-run onboarding flow for users who are not comfortable with CLI setup
- Persistent app settings with sensible defaults seeded from the legacy shell scripts
- Compatibility with existing MySQL Shell snapshot directories under `/opt/homebrew/var/db_snapshots/mysqlsh`
- Snapshot safety: dumps are written to a temp directory and only swapped in after success
- Restore safety: snapshots are validated before any database drop happens
- Automatic `local_infile` handling during restore when enabled in settings
- Plain CLI subcommands for direct use and scripting
- Live job log streaming in the TUI and persistent per-database log files under `_logs`

## Requirements

- Go 1.25.8 or newer to build this repo
- MySQL Shell: `mysqlsh`
- MySQL client tools: `mysql`, `mysqladmin`
- Homebrew if you want `dbgold` to start `mysql@8.0` for you
- A local MySQL instance or any MySQL setup reachable with the configured host/socket/login settings

## Installation

Build the binary:

```bash
go build -o dbgold ./cmd/dbgold
```

Optional: put it somewhere on your `PATH`:

```bash
ln -sf "$PWD/dbgold" ~/scripts/dbgold
```

Generate zsh completion:

```bash
dbgold completion zsh > ~/.zsh/completions/_dbgold
```

## First Run

Run:

```bash
dbgold
```

On the first launch, `dbgold` opens a guided setup flow before the dashboard. The form starts with defaults taken from the legacy shell scripts, so most local setups only need a quick review.

The saved settings file lives at:

```text
~/Library/Application Support/dbgold/settings.json
```

You can override that path with:

```bash
DBGOLD_CONFIG_PATH=/path/to/settings.json
```

## Usage

Launch the TUI dashboard:

```bash
dbgold
```

Direct commands:

```bash
dbgold snapshot [db]
dbgold restore [db]
dbgold list dbs
dbgold list snapshots
dbgold doctor
dbgold settings
dbgold completion zsh
```

Global flags:

```text
--yes
--no-tui
--debug
```

### Name Handling

- Exact names run immediately
- Missing names open the relevant picker in the TUI
- Partial names open the picker prefiltered in TUI mode
- In non-interactive mode, actions require an exact name

## TUI Controls

### Dashboard and Pickers

- `j` / `k` or arrow keys: move selection
- `/`: focus filter
- `enter`: select or confirm
- `esc`: go back or clear filter
- `r`: refresh
- `s`: open snapshot flow
- `R`: open restore flow
- `d`: open doctor screen
- `c`: open settings
- `q`: quit

### Running Jobs

- `ctrl+c`: request cancellation, then confirm

## Settings and Configuration

`dbgold` uses this precedence:

1. Built-in defaults
2. Saved settings file
3. Environment variables

That means the app is beginner-friendly by default, but still works well in custom or team-specific environments.

### Main Settings

- Snapshot root
- Log root
- MySQL Shell state directory
- MySQL host, port, socket, user, login path, password, or full MySQL Shell URI
- MySQL service name for Homebrew start prompts
- MySQL Shell thread count
- Compression
- Chunk size
- Deferred index behavior
- Skip binlog on restore
- Auto-enable `local_infile` on restore

### Supported Environment Variables

Supported runtime variables:

```text
MYSQL_SNAPSHOT_ROOT
MYSQL_LOG_ROOT
MYSQLSH_USER_CONFIG_HOME
MYSQLSH_HEARTBEAT_INTERVAL
MYSQL_START_TIMEOUT
MYSQLSH_URI
MYSQL_SOCKET
MYSQL_USER
MYSQL_PASSWORD
MYSQL_PWD
MYSQL_LOGIN_PATH
MYSQL_HOST
MYSQL_PORT
MYSQL_SERVICE_NAME
MYSQL_ASSUME_EMPTY_PASSWORD
MYSQLSH_THREADS
MYSQLSH_COMPRESSION
MYSQLSH_BYTES_PER_CHUNK
MYSQLSH_DEFER_TABLE_INDEXES
MYSQLSH_SKIP_BINLOG
MYSQLSH_AUTO_ENABLE_LOCAL_INFILE
```

App-specific variables:

```text
DBGOLD_CONFIG_PATH
DBGOLD_YES
DBGOLD_NO_TUI
DBGOLD_DEBUG
```

It reuses:

- Snapshot root layout: `/opt/homebrew/var/db_snapshots/mysqlsh/<database>`
- Log files:
  - `_logs/<db>.snapshot.log`
  - `_logs/<db>.restore.log`
- Existing `snapshot.info` files
- Existing MySQL Shell dump metadata such as `@.json`

## Safety Model

### Snapshot

- Verifies MySQL availability before running
- Uses a temp dump directory
- Replaces the existing snapshot only after a successful dump
- Keeps the old snapshot intact if the dump fails

### Restore

- Verifies the snapshot looks like a valid MySQL Shell dump before dropping anything
- Checks MySQL availability before running
- Can temporarily enable `GLOBAL local_infile` and restore it afterward
- Tries to reset `local_infile` even if restore work fails

## Logs

Each run writes a persistent log file:

```text
<snapshot-root>/_logs/<db>.snapshot.log
<snapshot-root>/_logs/<db>.restore.log
```

The TUI running screen also streams log output live.

## Examples

Take a snapshot without opening the full dashboard:

```bash
dbgold snapshot my_app --no-tui
```

Restore a snapshot non-interactively:

```bash
dbgold restore my_app --no-tui --yes
```

Inspect saved settings:

```bash
dbgold settings --no-tui
```

Use a different settings file:

```bash
DBGOLD_CONFIG_PATH=./dbgold.settings.json dbgold
```

## Why Use It

For small databases, `mysqldump` and SQL restore scripts can be good enough.

For larger local-development databases, that pattern tends to get slow because:

- plain SQL dumps are larger and slower to import
- restore time grows noticeably as data volume grows
- repeated drop-and-rebuild cycles become disruptive during normal development

`dbgold` uses MySQL Shell dump and load operations instead, which are typically much faster for larger local datasets and make snapshot/restore workflows more practical during day-to-day development.

## Troubleshooting

### MySQL is not reachable

- Run `dbgold doctor`
- Confirm your host, port, socket, login path, or URI in `dbgold settings`
- If you use Homebrew, confirm the configured service name matches your install

### Auth works in one machine but not another

- Check whether that machine should use a socket, host/port, login path, or full `MYSQLSH_URI`
- If the server uses a blank local password, keep `mysql_assume_empty_password` enabled
- If it does not, disable that setting and configure a login path or explicit password

### Restore fails on `local_infile`

- Enable `mysqlsh_auto_enable_local_infile` in settings, or
- Manually enable `GLOBAL local_infile` on the target server

## Development

Run tests:

```bash
go test ./...
```

Run the app:

```bash
go run ./cmd/dbgold
```

Key project directories:

```text
cmd/dbgold      CLI entrypoint
internal/app    TUI and Cobra command wiring
internal/core   config, metadata, discovery, orchestration
internal/execx  process execution and output streaming
```
