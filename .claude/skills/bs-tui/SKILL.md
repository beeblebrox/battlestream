---
name: bs-tui
description: Render the TUI dashboard to stdout for debugging
disable-model-invocation: true
allowed-tools: Bash(./battlestream *)
---

# bs-tui

Render the battlestream TUI to stdout for non-interactive debugging.

## Usage

`/bs-tui` — render at default width
`/bs-tui 120` — render at specified width

## Steps

1. If width argument provided: `./battlestream tui --dump --width <width>`
2. Otherwise: `./battlestream tui --dump`
3. Display the rendered output
