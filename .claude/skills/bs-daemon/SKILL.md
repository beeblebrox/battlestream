---
name: bs-daemon
description: "Start, stop, or restart the battlestream daemon"
disable-model-invocation: true
allowed-tools: Bash(ps *), Bash(kill *), Bash(pkill *), Bash(xargs *), Bash(./battlestream *), Bash(sleep *), Bash(grep *)
---

# bs-daemon

Manage the battlestream daemon process.

## Usage

`/bs-daemon start` | `/bs-daemon stop` | `/bs-daemon restart`

## Steps

### stop
1. Find running daemon: `ps aux | grep battlestream | grep daemon | grep -v grep`
2. Kill it: `pkill -f "battlestream daemon"` or `ps aux | grep "battlestream daemon" | grep -v grep | awk '{print $2}' | xargs -r kill -9`
3. Confirm it's gone

### start
1. Check if daemon is already running
2. Start: `./battlestream daemon > /tmp/battlestream-daemon.log 2>&1 &`
3. `sleep 1`
4. Verify it's running: `ps aux | grep "battlestream daemon" | grep -v grep`

### restart
1. Run stop steps
2. Run start steps

If no argument given, default to `restart`.
