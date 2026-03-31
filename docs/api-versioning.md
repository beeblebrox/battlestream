# API Versioning Policy

## REST API

- All endpoints are prefixed with `/v1/` (current stable version).
- **Breaking changes** (removed endpoints, changed request/response shapes, removed fields) go into a new prefix (e.g., `/v2/`).
- The prior major version is kept alive for **2 major application releases** after the new version ships, then removed.
- Non-breaking additions (new optional fields, new endpoints) may be added to the current version without a bump.

### What counts as a breaking change (REST)

- Removing or renaming an endpoint path.
- Removing or renaming a field in a response body.
- Changing the type of an existing field.
- Changing from optional to required on a request field.
- Changing HTTP method, status codes, or error shapes in a way that breaks existing callers.

## gRPC / Protobuf

- Proto fields are **additive only**. Never remove or renumber a field.
- Removed fields **must** be marked with `reserved` (both the number and the name) so they cannot be accidentally reused.
- New optional fields can be added to any message at any time without a version bump.
- Service method signatures may not change in a breaking way within a major version; add a new RPC instead.

### What counts as a breaking change (proto)

- Removing a field without adding a `reserved` entry.
- Renumbering any existing field.
- Changing a field's type.
- Removing a service method.
- Changing a method's request or response type to an incompatible shape.

## Deprecation Process

1. Mark the endpoint or field as deprecated:
   - REST: add a `Deprecation` response header and a note in the API docs.
   - Proto: add `[deprecated = true]` option and a comment referencing the replacement.
2. Keep the deprecated item working for **1 full major version** (i.e., until the next major version ships and itself becomes stable).
3. Remove it in the version after that.

## CI Enforcement

A `buf breaking` job in CI compares the current proto files against the `main` branch on every push and pull request. Any breaking proto change fails the check. See the `proto-breaking` job in `.github/workflows/ci.yml`.
