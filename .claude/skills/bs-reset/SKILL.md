---
name: bs-reset
description: Reset the BadgerDB database and reparse all Power.log history
disable-model-invocation: true
allowed-tools: Bash(echo *), Bash(./battlestream *)
---

# bs-reset

Reset the battlestream database and reparse all Power.log history.

## Steps

1. Confirm with the user before proceeding (data will be wiped)
2. Run: `echo "yes" | ./battlestream db-reset`
3. Run: `./battlestream reparse`
4. Report success/failure
