# Configuration

Config file is loaded from (in order):
1. `--config` flag path
2. `~/.battlestream/config.yaml`
3. `./config.yaml`
4. `/etc/battlestream/config.yaml`

Environment variables override file values. Prefix: `BS_`, separator `_`.
Example: `BS_API_GRPC_ADDR=127.0.0.1:50051`

## Full Reference

```yaml
hearthstone:
  # Path to Hearthstone install root. Auto-detected if blank.
  install_path: ""

  # Path to Hearthstone Logs/ directory. Derived from install_path if blank.
  log_path: ""

  # Automatically write required sections to log.config on daemon start.
  auto_patch_logconfig: true

storage:
  # Path to the BadgerDB data directory.
  db_path: "~/.battlestream/data"

output:
  # Enable JSON file output.
  enabled: true

  # Directory to write stat files to.
  path: "~/.battlestream/stats"

  # How often to flush current state to disk (milliseconds).
  write_interval_ms: 500

api:
  # gRPC server listen address.
  grpc_addr: "127.0.0.1:50051"

  # REST/WebSocket/SSE server listen address.
  rest_addr: "127.0.0.1:8080"

  # If set, require 'Authorization: Bearer <key>' on all API requests.
  # Leave empty to disable auth (recommended for localhost-only use).
  api_key: ""

logging:
  # Log level: debug, info, warn, error
  level: "info"

  # Log to this file in addition to stderr. Empty = stderr only.
  file: ""
```

## Environment Variables

| Variable | Config Key | Example |
|---|---|---|
| `BS_HEARTHSTONE_INSTALL_PATH` | `hearthstone.install_path` | `/opt/hearthstone` |
| `BS_HEARTHSTONE_LOG_PATH` | `hearthstone.log_path` | `/opt/hearthstone/Logs` |
| `BS_STORAGE_DB_PATH` | `storage.db_path` | `~/.battlestream/data` |
| `BS_OUTPUT_PATH` | `output.path` | `~/obs-sources/bs-stats` |
| `BS_API_GRPC_ADDR` | `api.grpc_addr` | `0.0.0.0:50051` |
| `BS_API_REST_ADDR` | `api.rest_addr` | `0.0.0.0:8080` |
| `BS_API_API_KEY` | `api.api_key` | `s3cr3t` |
| `BS_LOGGING_LEVEL` | `logging.level` | `debug` |
