---
name: bs-build
description: Build and optionally test the battlestream binary
disable-model-invocation: true
allowed-tools: Bash(go *)
---

# bs-build

Build and optionally test the battlestream binary.

## Usage

`/bs-build` — build only
`/bs-build test` — build + run tests

## Steps

### build (default)
1. Run: `go build -o ./battlestream ./cmd/battlestream/`
2. Report success/failure and binary size

### test
1. Run: `go build -o ./battlestream ./cmd/battlestream/`
2. Run: `go test ./...`
3. Report results
