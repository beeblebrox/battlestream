# Signed Releases & Auto-Update Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate to GoReleaser with cosign signing, add macOS notarization and Windows Authenticode signing, distribute via package managers, and add a self-update mechanism so users never hit Gatekeeper/SmartScreen warnings or need to manually download updates.

**Architecture:** Five phases, each independently shippable. Phase 1 replaces the custom release workflow with GoReleaser + cosign keyless signing + SBOM generation. Phase 2 adds self-update (notification + `battlestream update` command). Phase 3 adds macOS code signing + notarization via GoReleaser's Quill integration (runs on Linux CI). Phase 4 adds Windows Authenticode signing via osslsigncode (OV cert) or Azure Artifact Signing. Phase 5 adds Homebrew tap and Scoop bucket for package manager distribution.

**Tech Stack:** GoReleaser v2, cosign/sigstore, Syft (SBOM), creativeprojects/go-selfupdate, Quill (macOS signing), osslsigncode (Windows signing), Homebrew, Scoop

---

## Phase Overview

| Phase | What | Cost | Solves |
|-------|------|------|--------|
| 1 | GoReleaser + cosign + SBOM | Free | Supply chain trust, reproducible releases |
| 2 | Auto-update notification + command | Free | Users stay current without browser downloads |
| 3 | macOS code signing + notarization | $99/yr Apple Developer ID | Gatekeeper "unidentified developer" warnings |
| 4 | Windows Authenticode signing | ~$80-120/yr OV cert or $10/mo Azure | SmartScreen "unknown publisher" warnings |
| 5 | Homebrew tap + Scoop bucket | Free | Package manager installs bypass OS warnings entirely |

**Recommended execution order:** Phase 1 -> Phase 5 -> Phase 2 -> Phase 3 -> Phase 4

Phase 5 before 2-4 because Homebrew/Scoop installs bypass Gatekeeper/SmartScreen entirely and are free. Most technical users prefer package managers. Phases 3 and 4 require paid accounts and are primarily for users who download binaries directly.

---

## Phase 1: GoReleaser Migration + Cosign Signing

### Task 1: Create .goreleaser.yaml

**Files:**
- Create: `.goreleaser.yaml`

**Step 1: Write the GoReleaser config**

```yaml
version: 2

project_name: battlestream

before:
  hooks:
    - go mod tidy

builds:
  - id: battlestream
    main: ./cmd/battlestream
    binary: battlestream
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w -X main.version={{ .Version }}
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64

universal_binaries:
  - id: battlestream-universal
    ids:
      - battlestream
    name_template: battlestream
    replace: false

archives:
  - id: default
    formats:
      - tar.gz
    format_overrides:
      - goos: windows
        formats:
          - zip
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
      {{- if eq .Arch "all" }}_universal{{ end }}

checksum:
  name_template: checksums.txt
  algorithm: sha256

sboms:
  - artifacts: archive

signs:
  - cmd: cosign
    certificate: "${artifact}.pem"
    args:
      - sign-blob
      - --output-signature=${signature}
      - --output-certificate=${certificate}
      - ${artifact}
    artifacts: checksum
    output: true

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - Merge pull request

release:
  github:
    owner: beeblebrox
    name: battlestream
  prerelease: auto
```

**Step 2: Test locally (dry-run)**

Run: `goreleaser check` (validates config syntax)

If goreleaser is not installed:
```bash
go install github.com/goreleaser/goreleaser/v2@latest
```

Then: `goreleaser release --snapshot --clean`

This builds all artifacts locally without publishing. Verify the `dist/` directory contains:
- `battlestream_<version>_linux_amd64.tar.gz`
- `battlestream_<version>_linux_arm64.tar.gz`
- `battlestream_<version>_windows_amd64.zip`
- `battlestream_<version>_darwin_all.tar.gz` (universal)
- `checksums.txt`

**Step 3: Commit**

```bash
git add .goreleaser.yaml
git commit -m "feat: add GoReleaser config with cosign signing and SBOM"
```

---

### Task 2: Replace release workflow with GoReleaser

**Files:**
- Modify: `.github/workflows/release.yml`

**Step 1: Rewrite the release workflow**

Replace the entire `release.yml` with:

```yaml
name: Release

on:
  push:
    tags: ['v*']
  workflow_dispatch:
    inputs:
      tag:
        description: 'Tag to release (e.g. v0.14.0-beta)'
        required: true

permissions:
  contents: write
  id-token: write  # required for cosign keyless signing

jobs:
  ci:
    name: CI
    uses: ./.github/workflows/ci.yml

  release:
    name: Build & Release
    needs: ci
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Install cosign
        uses: sigstore/cosign-installer@v3

      - name: Install Syft (SBOM generator)
        uses: anchore/sbom-action/download-syft@v0

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**Step 2: Add `dist/` to .gitignore if not already present**

```bash
echo "dist/" >> .gitignore
```

**Step 3: Commit**

```bash
git add .github/workflows/release.yml .gitignore
git commit -m "feat: migrate release workflow to GoReleaser with cosign + SBOM

Replaces custom 136-line build+archive+release shell with GoReleaser.
Adds cosign keyless signing of checksums and Syft SBOM generation.
Release artifacts now follow GoReleaser naming convention."
```

---

### Task 3: Test the new release pipeline

**Step 1: Push and tag a release candidate**

```bash
git push origin main
# Wait for CI to pass
gh run watch
# Then tag
git tag -a v0.14.0-beta -m "v0.14.0-beta: GoReleaser migration"
git push origin v0.14.0-beta
```

**Step 2: Watch the Release workflow**

```bash
gh run list --limit 3
gh run watch <release-run-id>
```

**Step 3: Verify the release**

```bash
gh release view v0.14.0-beta
```

Expected assets:
- `battlestream_0.14.0-beta_linux_amd64.tar.gz`
- `battlestream_0.14.0-beta_linux_arm64.tar.gz`
- `battlestream_0.14.0-beta_windows_amd64.zip`
- `battlestream_0.14.0-beta_darwin_all.tar.gz`
- `checksums.txt`
- `checksums.txt.sig` (cosign signature)
- `checksums.txt.pem` (cosign certificate)
- SBOM files

**Step 4: Verify cosign signature works**

```bash
cosign verify-blob \
  --certificate-identity "https://github.com/beeblebrox/battlestream/.github/workflows/release.yml@refs/tags/v0.14.0-beta" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  checksums.txt
```

---

## Phase 2: Auto-Update

### Task 4: Add update check package

**Files:**
- Create: `internal/update/update.go`
- Create: `internal/update/update_test.go`

**Step 1: Write the update check module**

```go
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	checkInterval = 24 * time.Hour
	repoSlug      = "beeblebrox/battlestream"
	stateFile     = "update-state.yaml"
)

type ReleaseInfo struct {
	Version string `json:"tag_name" yaml:"version"`
	URL     string `json:"html_url" yaml:"url"`
}

type state struct {
	CheckedAt time.Time `yaml:"checked_at"`
	Latest    string    `yaml:"latest_version"`
	URL       string    `yaml:"release_url"`
}

// CheckResult is returned from CheckForUpdate.
type CheckResult struct {
	NewVersion string
	URL        string
}

// ShouldCheck returns true if an update check should be performed.
func ShouldCheck(stateDir string) bool {
	if os.Getenv("BS_NO_UPDATE_CHECK") != "" {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	s, err := readState(stateDir)
	if err != nil {
		return true // no state file = never checked
	}
	return time.Since(s.CheckedAt) > checkInterval
}

// CheckForUpdate queries GitHub for the latest release and returns
// a result if a newer version is available.
func CheckForUpdate(stateDir, currentVersion string) (*CheckResult, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %s", resp.Status)
	}

	var rel ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	// Save state regardless of version comparison.
	_ = writeState(stateDir, state{
		CheckedAt: time.Now(),
		Latest:    rel.Version,
		URL:       rel.URL,
	})

	if !isNewer(rel.Version, currentVersion) {
		return nil, nil
	}

	return &CheckResult{
		NewVersion: rel.Version,
		URL:        rel.URL,
	}, nil
}

// AssetName returns the expected release asset name for the current platform.
func AssetName(version string) string {
	os := runtime.GOOS
	arch := runtime.GOARCH
	if os == "darwin" {
		arch = "all" // universal binary
	}
	ext := "tar.gz"
	if os == "windows" {
		ext = "zip"
	}
	v := strings.TrimPrefix(version, "v")
	return fmt.Sprintf("battlestream_%s_%s_%s.%s", v, os, arch, ext)
}

func isNewer(latest, current string) bool {
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")
	if current == "dev" || current == "" {
		return false
	}
	return latest != current && latest > current
}

func statePath(dir string) string {
	return filepath.Join(dir, stateFile)
}

func readState(dir string) (*state, error) {
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		return nil, err
	}
	var s state
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func writeState(dir string, s state) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), data, 0o644)
}
```

**Step 2: Write tests**

```go
package update

import (
	"os"
	"testing"
	"time"
)

func TestShouldCheck_NoState(t *testing.T) {
	dir := t.TempDir()
	if !ShouldCheck(dir) {
		t.Error("expected true when no state file exists")
	}
}

func TestShouldCheck_RecentCheck(t *testing.T) {
	dir := t.TempDir()
	_ = writeState(dir, state{CheckedAt: time.Now()})
	if ShouldCheck(dir) {
		t.Error("expected false when checked recently")
	}
}

func TestShouldCheck_StaleCheck(t *testing.T) {
	dir := t.TempDir()
	_ = writeState(dir, state{CheckedAt: time.Now().Add(-25 * time.Hour)})
	if !ShouldCheck(dir) {
		t.Error("expected true when last check >24h ago")
	}
}

func TestShouldCheck_EnvDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BS_NO_UPDATE_CHECK", "1")
	if ShouldCheck(dir) {
		t.Error("expected false when BS_NO_UPDATE_CHECK set")
	}
}

func TestShouldCheck_CI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CI", "true")
	if ShouldCheck(dir) {
		t.Error("expected false in CI")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v0.14.0", "v0.13.0", true},
		{"v0.13.0", "v0.13.0", false},
		{"v0.12.0", "v0.13.0", false},
		{"v0.14.0-beta", "v0.13.0-beta", true},
		{"v0.14.0", "dev", false},
		{"v0.14.0", "", false},
	}
	for _, tt := range tests {
		if got := isNewer(tt.latest, tt.current); got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	name := AssetName("v0.14.0-beta")
	if name == "" {
		t.Fatal("empty asset name")
	}
	// Just verify it has the version and extension
	if !contains(name, "0.14.0-beta") {
		t.Errorf("asset name %q missing version", name)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := state{
		CheckedAt: time.Now().Truncate(time.Second),
		Latest:    "v0.14.0",
		URL:       "https://example.com",
	}
	if err := writeState(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := readState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Latest != want.Latest || got.URL != want.URL {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
```

**Step 3: Run tests**

Run: `go test -count=1 ./internal/update/`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/update/
git commit -m "feat: add update check package with 24h throttle and state persistence"
```

---

### Task 5: Add `update` subcommand and background notification

**Files:**
- Create: `cmd/battlestream/update.go`
- Modify: `cmd/battlestream/main.go`

**Step 1: Add the `update` command**

Add `creativeprojects/go-selfupdate` dependency:
```bash
go get github.com/creativeprojects/go-selfupdate
```

Create `cmd/battlestream/update.go`:
```go
package main

import (
	"fmt"

	selfupdate "github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update battlestream to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
		if err != nil {
			return fmt.Errorf("create source: %w", err)
		}
		updater, err := selfupdate.NewUpdater(selfupdate.Config{
			Source:    source,
			Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
		})
		if err != nil {
			return fmt.Errorf("create updater: %w", err)
		}

		latest, found, err := updater.DetectLatest(cmd.Context(), selfupdate.ParseSlug("beeblebrox/battlestream"))
		if err != nil {
			return fmt.Errorf("detect latest: %w", err)
		}
		if !found {
			fmt.Println("No release found.")
			return nil
		}

		current := version
		if latest.LessOrEqual(current) {
			fmt.Printf("Already up to date: %s\n", current)
			return nil
		}

		fmt.Printf("Updating %s -> %s ...\n", current, latest.Version())
		exe, err := selfupdate.ExecutablePath()
		if err != nil {
			return fmt.Errorf("executable path: %w", err)
		}
		if err := updater.UpdateTo(cmd.Context(), latest, exe); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
		fmt.Printf("Updated to %s\n", latest.Version())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
```

**Step 2: Add background update notification to main.go**

In `main.go`, add the background check goroutine before `rootCmd.Execute()`. The exact integration depends on your current `main()` structure. Add to the imports and the pre-execute section:

```go
import (
	"battlestream.fixates.io/internal/update"
	"golang.org/x/term"
)

// In main() or the root command's PersistentPreRun, before Execute:
type updateResult struct {
	result *update.CheckResult
}

var updateCh chan updateResult

func startUpdateCheck() {
	stateDir := filepath.Join(configDir(), "profiles", "default")
	if !update.ShouldCheck(stateDir) {
		return
	}
	updateCh = make(chan updateResult, 1)
	go func() {
		res, _ := update.CheckForUpdate(stateDir, version)
		updateCh <- updateResult{result: res}
	}()
}

func printUpdateNotification() {
	if updateCh == nil {
		return
	}
	select {
	case r := <-updateCh:
		if r.result != nil && term.IsTerminal(int(os.Stderr.Fd())) {
			fmt.Fprintf(os.Stderr, "\nUpdate available: %s -> %s\n", version, r.result.NewVersion)
			fmt.Fprintf(os.Stderr, "Run \"battlestream update\" to upgrade.\n\n")
		}
	default:
	}
}
```

Call `startUpdateCheck()` early in main, and `printUpdateNotification()` after `rootCmd.Execute()` returns.

**Step 3: Run tests and build**

```bash
go test -count=1 ./...
go build ./cmd/battlestream
```

**Step 4: Commit**

```bash
git add cmd/battlestream/update.go cmd/battlestream/main.go go.mod go.sum
git commit -m "feat: add 'battlestream update' command and background update notification

Uses creativeprojects/go-selfupdate with checksum validation.
Background check runs once per 24h, prints to stderr on TTY.
Disabled via BS_NO_UPDATE_CHECK=1 or in CI."
```

---

## Phase 3: macOS Code Signing + Notarization

> **Prerequisite:** Apple Developer Program membership ($99/yr) and a Developer ID Application certificate exported as .p12.

### Task 6: Add macOS notarization to GoReleaser

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `.github/workflows/release.yml`

**Step 1: Add notarize block to .goreleaser.yaml**

GoReleaser v2 supports cross-platform macOS signing via Quill (runs on Linux). Add after the `signs:` block:

```yaml
notarize:
  - macos:
      - enabled: '{{ isEnvSet "MACOS_SIGN_P12" }}'
        sign:
          certificate: "{{ .Env.MACOS_SIGN_P12 }}"
          password: "{{ .Env.MACOS_SIGN_PASSWORD }}"
        notarize:
          issuer_id: "{{ .Env.MACOS_NOTARY_ISSUER_ID }}"
          key_id: "{{ .Env.MACOS_NOTARY_KEY_ID }}"
          key: "{{ .Env.MACOS_NOTARY_KEY }}"
```

**Step 2: Add secrets to the release workflow env**

In `.github/workflows/release.yml`, add to the GoReleaser step's `env:`:

```yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          MACOS_SIGN_P12: ${{ secrets.MACOS_SIGN_P12 }}
          MACOS_SIGN_PASSWORD: ${{ secrets.MACOS_SIGN_PASSWORD }}
          MACOS_NOTARY_ISSUER_ID: ${{ secrets.MACOS_NOTARY_ISSUER_ID }}
          MACOS_NOTARY_KEY_ID: ${{ secrets.MACOS_NOTARY_KEY_ID }}
          MACOS_NOTARY_KEY: ${{ secrets.MACOS_NOTARY_KEY }}
```

**Step 3: Store secrets in GitHub**

These must be configured in the GitHub repository settings (Settings > Secrets > Actions):

| Secret | Value |
|--------|-------|
| `MACOS_SIGN_P12` | Base64-encoded Developer ID Application .p12 file |
| `MACOS_SIGN_PASSWORD` | Password for the .p12 |
| `MACOS_NOTARY_ISSUER_ID` | App Store Connect API issuer UUID |
| `MACOS_NOTARY_KEY_ID` | App Store Connect API key ID |
| `MACOS_NOTARY_KEY` | Contents of the AuthKey_XXXX.p8 file |

Generate the App Store Connect API key at: https://appstoreconnect.apple.com/access/integrations/api

**Step 4: Commit and test**

```bash
git add .goreleaser.yaml .github/workflows/release.yml
git commit -m "feat: add macOS code signing and notarization via GoReleaser/Quill

Uses cross-platform Quill signer (runs on Linux CI).
Requires MACOS_SIGN_P12 and MACOS_NOTARY_* secrets.
Gracefully skipped if secrets not set (isEnvSet guard)."
```

Tag a test release and verify the macOS binary passes Gatekeeper:
```bash
# On a Mac, after downloading:
spctl --assess --type execute ./battlestream
# Expected: accepted
```

---

## Phase 4: Windows Authenticode Signing

> **Prerequisite:** Either an OV code signing certificate (~$80/yr from Sectigo via reseller) OR an Azure Artifact Signing account ($9.99/mo).

### Task 7: Add Windows signing to GoReleaser

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `.github/workflows/release.yml`
- Create: `scripts/sign-windows.sh`

**Option A: OV certificate with osslsigncode (recommended for simplicity)**

**Step 1: Create signing script**

Create `scripts/sign-windows.sh`:
```bash
#!/bin/bash
# Sign a Windows PE binary with osslsigncode.
# Called by GoReleaser post-build hook.
set -euo pipefail

FILE="$1"
OS="$2"

if [ "$OS" != "windows" ]; then
  exit 0
fi

if [ -z "${WIN_CERT_PFX:-}" ]; then
  echo "WIN_CERT_PFX not set, skipping Windows signing"
  exit 0
fi

# Decode cert if base64-encoded
CERT_FILE=$(mktemp)
echo "$WIN_CERT_PFX" | base64 -d > "$CERT_FILE"
trap "rm -f $CERT_FILE" EXIT

osslsigncode sign \
  -pkcs12 "$CERT_FILE" \
  -pass "$WIN_CERT_PASSWORD" \
  -h sha256 \
  -ts http://timestamp.digicert.com \
  -in "$FILE" \
  -out "${FILE}.signed"

mv "${FILE}.signed" "$FILE"
```

```bash
chmod +x scripts/sign-windows.sh
```

**Step 2: Add post-build hook to .goreleaser.yaml**

Add to the `builds:` section:
```yaml
builds:
  - id: battlestream
    # ... existing config ...
    hooks:
      post:
        - cmd: scripts/sign-windows.sh {{ .Path }} {{ .Os }}
          output: true
```

**Step 3: Install osslsigncode in the release workflow**

Add before the GoReleaser step:
```yaml
      - name: Install osslsigncode
        run: sudo apt-get install -y osslsigncode
```

And add secrets to the GoReleaser env:
```yaml
          WIN_CERT_PFX: ${{ secrets.WIN_CERT_PFX }}
          WIN_CERT_PASSWORD: ${{ secrets.WIN_CERT_PASSWORD }}
```

**Step 4: Commit**

```bash
git add scripts/sign-windows.sh .goreleaser.yaml .github/workflows/release.yml
git commit -m "feat: add Windows Authenticode signing via osslsigncode

Post-build hook signs .exe with OV certificate.
Gracefully skipped if WIN_CERT_PFX not set."
```

**Option B: Azure Artifact Signing ($9.99/mo)**

Replace the osslsigncode approach with the Azure Trusted Signing GitHub Action. This requires a separate Windows runner job. Add after the GoReleaser release job:

```yaml
  sign-windows:
    name: Sign Windows Binary
    needs: release
    runs-on: windows-latest
    permissions:
      id-token: write
      contents: write
    steps:
      - name: Download Windows asset
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          gh release download ${{ github.ref_name }} -p "*windows_amd64.zip" -D .

      - name: Extract binary
        run: Expand-Archive -Path *.zip -DestinationPath extracted

      - name: Azure Login
        uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: Sign with Azure Artifact Signing
        uses: azure/trusted-signing-action@v0.5.0
        with:
          azure-tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          azure-client-id: ${{ secrets.AZURE_CLIENT_ID }}
          azure-client-secret: ${{ secrets.AZURE_CLIENT_SECRET }}
          endpoint: https://eus.codesigning.azure.net/
          trusted-signing-account-name: your-account-name
          certificate-profile-name: your-profile-name
          files-folder: extracted
          files-folder-filter: exe
          file-digest: SHA256
          timestamp-rfc3161: http://timestamp.acs.microsoft.com
          timestamp-digest: SHA256

      - name: Re-package and upload
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          # Re-zip and upload the signed binary, replacing the unsigned one
          Compress-Archive -Path extracted\* -DestinationPath signed.zip
          gh release upload ${{ github.ref_name }} signed.zip --clobber
```

---

## Phase 5: Package Manager Distribution

### Task 8: Create Homebrew tap

**Files:**
- Modify: `.goreleaser.yaml`

**Step 1: Create the tap repository**

Create a new GitHub repo: `beeblebrox/homebrew-tap` (must have `homebrew-` prefix).

**Step 2: Add brews block to .goreleaser.yaml**

```yaml
brews:
  - repository:
      owner: beeblebrox
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    homepage: https://github.com/beeblebrox/battlestream
    description: Hearthstone Battlegrounds stat tracker
    license: MIT
    install: |
      bin.install "battlestream"
    test: |
      system "#{bin}/battlestream", "version"
```

**Step 3: Add token secret to release workflow**

Create a GitHub PAT with `repo` scope for the tap repo. Store as `HOMEBREW_TAP_TOKEN` secret.

Add to GoReleaser env:
```yaml
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

Users install with:
```bash
brew tap beeblebrox/tap
brew install battlestream
```

**Step 4: Commit**

```bash
git add .goreleaser.yaml .github/workflows/release.yml
git commit -m "feat: add Homebrew tap for macOS/Linux package manager distribution"
```

---

### Task 9: Create Scoop bucket

**Files:**
- Modify: `.goreleaser.yaml`

**Step 1: Create the bucket repository**

Create a new GitHub repo: `beeblebrox/scoop-bucket`.

**Step 2: Add scoops block to .goreleaser.yaml**

```yaml
scoops:
  - repository:
      owner: beeblebrox
      name: scoop-bucket
      token: "{{ .Env.SCOOP_BUCKET_TOKEN }}"
    homepage: https://github.com/beeblebrox/battlestream
    description: Hearthstone Battlegrounds stat tracker
    license: MIT
```

**Step 3: Add token secret**

Reuse the same PAT or create a new one. Store as `SCOOP_BUCKET_TOKEN`.

Users install with:
```powershell
scoop bucket add battlestream https://github.com/beeblebrox/scoop-bucket
scoop install battlestream
```

**Step 4: Commit**

```bash
git add .goreleaser.yaml .github/workflows/release.yml
git commit -m "feat: add Scoop bucket for Windows package manager distribution"
```

---

## Summary: What Each Phase Eliminates

| User Problem | Phase that Fixes It |
|---|---|
| macOS "unidentified developer" Gatekeeper popup | Phase 3 (notarization) or Phase 5 (Homebrew) |
| Windows SmartScreen "unknown publisher" warning | Phase 4 (Authenticode) or Phase 5 (Scoop) |
| Browser "this file may be dangerous" warning | Phase 5 (package managers bypass browser entirely) |
| Users running outdated versions | Phase 2 (auto-update) |
| No supply chain verification | Phase 1 (cosign + SBOM) |
| Manual release process | Phase 1 (GoReleaser) |
