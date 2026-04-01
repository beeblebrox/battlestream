#!/usr/bin/env bash
# provision-repo.sh — Create a private GitHub repo using a GitHub App installation token.
#
# Usage:
#   ./provision-repo.sh <APP_ID> <INSTALL_ID> <KEY_PEM_PATH> <REPO_NAME> [DESCRIPTION]
#
# Requirements:
#   - openssl (for JWT signing)
#   - curl
#   - python3 (for base64url encoding helper)
#
# The repo is created as private under the 'beeblebrox' GitHub account.

set -euo pipefail

GITHUB_OWNER="beeblebrox"

usage() {
  echo "Usage: $0 <APP_ID> <INSTALL_ID> <KEY_PEM_PATH> <REPO_NAME> [DESCRIPTION]"
  echo
  echo "  APP_ID       GitHub App numeric ID"
  echo "  INSTALL_ID   GitHub App installation ID (from /settings/installations/<id>)"
  echo "  KEY_PEM_PATH Path to the GitHub App private key (.pem)"
  echo "  REPO_NAME    Repository name to create (e.g. 'battlestream-onboarding')"
  echo "  DESCRIPTION  Optional repo description"
  exit 1
}

[[ $# -lt 4 ]] && usage

APP_ID="$1"
INSTALL_ID="$2"
KEY_PEM="$3"
REPO_NAME="$4"
DESCRIPTION="${5:-}"

if [[ ! -f "$KEY_PEM" ]]; then
  echo "Error: key file not found: $KEY_PEM" >&2
  exit 1
fi

# -----------------------------------------------------------------------
# 1. Generate a JWT for the GitHub App (valid for 60 seconds)
# -----------------------------------------------------------------------
now=$(date +%s)
iat=$((now - 60))   # issued-at slightly in the past to avoid clock skew
exp=$((now + 600))  # expires in 10 minutes (max allowed by GitHub is 10 min)

# base64url-encode (no padding) helper
b64url() {
  python3 -c "
import base64, sys
data = sys.stdin.buffer.read()
print(base64.urlsafe_b64encode(data).rstrip(b'=').decode())
"
}

header=$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)
payload=$(printf '{"iat":%d,"exp":%d,"iss":"%s"}' "$iat" "$exp" "$APP_ID" | b64url)

signing_input="${header}.${payload}"
signature=$(printf '%s' "$signing_input" | openssl dgst -sha256 -sign "$KEY_PEM" | b64url)

JWT="${signing_input}.${signature}"

# -----------------------------------------------------------------------
# 2. Exchange JWT for an installation access token
# -----------------------------------------------------------------------
echo "Requesting installation access token for installation $INSTALL_ID..."
token_response=$(curl -s -X POST \
  -H "Authorization: Bearer $JWT" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/app/installations/${INSTALL_ID}/access_tokens")

INSTALL_TOKEN=$(echo "$token_response" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('token',''))")

if [[ -z "$INSTALL_TOKEN" ]]; then
  echo "Error: failed to get installation token." >&2
  echo "Response: $token_response" >&2
  exit 1
fi

echo "Got installation token."

# -----------------------------------------------------------------------
# 3. Create the private repository
# -----------------------------------------------------------------------
echo "Creating private repo '${GITHUB_OWNER}/${REPO_NAME}'..."
create_payload=$(python3 -c "
import json
print(json.dumps({
  'name': '${REPO_NAME}',
  'description': '${DESCRIPTION}',
  'private': True,
  'auto_init': True,
}))
")

create_response=$(curl -s -X POST \
  -H "Authorization: Bearer $INSTALL_TOKEN" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/user/repos" \
  -d "$create_payload")

repo_url=$(echo "$create_response" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('html_url',''))")
clone_url=$(echo "$create_response" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('clone_url',''))")

if [[ -z "$repo_url" ]]; then
  echo "Error: repo creation failed." >&2
  echo "Response: $create_response" >&2
  exit 1
fi

echo ""
echo "Success!"
echo "  HTML URL:  $repo_url"
echo "  Clone URL: $clone_url"
