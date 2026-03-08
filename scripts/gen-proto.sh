#!/usr/bin/env bash
# Generate Go code from proto definitions.
#
# Prerequisites (install once):
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#   go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
#   go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
#
# protoc is downloaded automatically to ~/.local/bin if not found.
# googleapis is cloned to ~/src/googleapis if not found.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# --- protoc ---
PROTOC="${PROTOC:-$(command -v protoc 2>/dev/null || echo "${HOME}/bin/protoc")}"
if [[ ! -x "${PROTOC}" ]]; then
    echo "protoc not found. Downloading..."
    PROTOC_VERSION="29.3"
    PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip"
    PROTOC_URL="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"
    TMP=$(mktemp -d)
    curl -fsSL "${PROTOC_URL}" -o "${TMP}/protoc.zip"
    unzip -q "${TMP}/protoc.zip" -d "${TMP}/protoc-dist"
    mkdir -p "${HOME}/bin"
    cp "${TMP}/protoc-dist/bin/protoc" "${HOME}/bin/protoc"
    chmod +x "${HOME}/bin/protoc"
    PROTOC_INCLUDE="${TMP}/protoc-dist/include"
    PROTOC="${HOME}/bin/protoc"
    echo "protoc installed at ${PROTOC}"
else
    # Find bundled includes: check next to binary first (~/bin/include),
    # then standard layout (../include relative to binary dir).
    PROTOC_BIN_DIR="$(dirname "${PROTOC}")"
    PROTOC_INCLUDE="${PROTOC_BIN_DIR}/include"
    if [[ ! -d "${PROTOC_INCLUDE}/google/protobuf" ]]; then
        PROTOC_INCLUDE="${PROTOC_BIN_DIR}/../include"
    fi
    if [[ ! -d "${PROTOC_INCLUDE}/google/protobuf" ]]; then
        echo "ERROR: protoc well-known type includes not found. Run: /gen-proto install"
        exit 1
    fi
fi

# --- googleapis ---
GOOGLEAPIS="${GOOGLEAPIS:-${HOME}/src/googleapis}"
if [[ ! -d "${GOOGLEAPIS}/google/api" ]]; then
    echo "Cloning googleapis (sparse: google/api)..."
    mkdir -p "$(dirname "${GOOGLEAPIS}")"
    git clone --depth=1 --filter=blob:none --sparse \
        https://github.com/googleapis/googleapis "${GOOGLEAPIS}"
    (cd "${GOOGLEAPIS}" && git sparse-checkout set google/api)
fi

# --- grpc-gateway includes ---
GWMOD="$(go env GOPATH)/pkg/mod/$(go list -m -f '{{.Path}}@{{.Version}}' \
    github.com/grpc-ecosystem/grpc-gateway/v2 2>/dev/null || echo 'github.com/grpc-ecosystem/grpc-gateway/v2@v2.28.0')"

# --- output ---
OUT_DIR="${ROOT}/internal/api/grpc/gen"
DOCS_DIR="${ROOT}/docs"
mkdir -p "${OUT_DIR}" "${DOCS_DIR}"

export PATH="${HOME}/go/bin:${PATH}"

echo "Running protoc..."
"${PROTOC}" \
    -I "${ROOT}/proto" \
    -I "${GOOGLEAPIS}" \
    -I "${GWMOD}" \
    -I "${PROTOC_INCLUDE}" \
    --go_out="${OUT_DIR}" \
    --go_opt=paths=source_relative \
    --go-grpc_out="${OUT_DIR}" \
    --go-grpc_opt=paths=source_relative \
    --grpc-gateway_out="${OUT_DIR}" \
    --grpc-gateway_opt=paths=source_relative \
    battlestream/v1/game.proto \
    battlestream/v1/stats.proto \
    battlestream/v1/service.proto

echo "Proto generation complete -> ${OUT_DIR}"
echo "Run 'go build ./...' to verify."
