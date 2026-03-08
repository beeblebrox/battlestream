---
name: gen-proto
description: Install protoc toolchain locally and regenerate proto Go code
---

# gen-proto

Install the protoc compiler and Go plugins locally (if not already present), then regenerate the Go code from proto definitions.

## Usage

`/gen-proto` — install deps if needed and regenerate
`/gen-proto install` — only install/verify the toolchain
`/gen-proto generate` — only regenerate (assumes toolchain is present)

## Installation targets

All tools are installed to `~/bin` and `~/go/bin` (user-local, no root needed).

- **protoc** v29.3 — downloaded from GitHub releases to `~/bin/protoc`, with well-known type includes at `~/bin/include/`
- **protoc-gen-go** — `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`
- **protoc-gen-go-grpc** — `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`
- **protoc-gen-grpc-gateway** — `go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest`
- **googleapis** — sparse clone of `google/api` to `~/src/googleapis`

## Steps

### install

1. Check if `~/bin/protoc` exists and is executable. If not:
   - Download `protoc-29.3-linux-x86_64.zip` from GitHub releases
   - Extract binary to `~/bin/protoc` and includes to `~/bin/include/`
   - Verify: `~/bin/protoc --version` should print `libprotoc 29.3`
2. Check if `~/bin/include/google/protobuf/descriptor.proto` exists. If not:
   - Re-extract from the same zip (the includes dir was missed on initial install)
3. Install Go protoc plugins:
   ```
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
   ```
4. Check if `~/src/googleapis/google/api` exists. If not:
   ```
   git clone --depth=1 --filter=blob:none --sparse https://github.com/googleapis/googleapis ~/src/googleapis
   cd ~/src/googleapis && git sparse-checkout set google/api
   ```
5. Report installed versions and paths.

### generate

1. Set environment:
   ```
   PROTOC=~/bin/protoc
   PROTOC_INCLUDE=~/bin/include
   GOOGLEAPIS=~/src/googleapis
   GWMOD=$(go env GOPATH)/pkg/mod/$(go list -m -f '{{.Path}}@{{.Version}}' github.com/grpc-ecosystem/grpc-gateway/v2)
   OUT=internal/api/grpc/gen
   ```
2. Run protoc:
   ```
   $PROTOC \
     -I proto \
     -I $GOOGLEAPIS \
     -I "$GWMOD" \
     -I $PROTOC_INCLUDE \
     --go_out=$OUT --go_opt=paths=source_relative \
     --go-grpc_out=$OUT --go-grpc_opt=paths=source_relative \
     --grpc-gateway_out=$OUT --grpc-gateway_opt=paths=source_relative \
     battlestream/v1/game.proto \
     battlestream/v1/stats.proto \
     battlestream/v1/service.proto
   ```
3. Verify: `go build ./...`
4. Show `git diff --stat internal/api/grpc/gen/` to summarize changes.

### default (no args)

Run install, then generate.
