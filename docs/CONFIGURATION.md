# Configuration

Config file is loaded from (in order):
1. `--config` flag path
2. `~/.battlestream/config.yaml`
3. `./config.yaml`
4. `/etc/battlestream/config.yaml`

## Profiles

battlestream supports multiple named profiles, one per Hearthstone installation.
This is useful when running multiple Wine/Proton prefixes or having both a native
and a Wine install.

Use `--profile <name>` to select a profile when starting the daemon or TUI.
If only one profile is configured it is selected automatically.
The `active_profile` key sets the default when no `--profile` flag is given.

Run the interactive wizard to set up profiles:

```
battlestream discover
```

## Full Reference

```yaml
# Which profile to use when --profile is not specified and multiple exist.
active_profile: "main"

# One entry per Hearthstone installation.
profiles:
  main:
    hearthstone:
      # Path to Hearthstone install root. Auto-detected if blank.
      install_path: ""

      # Path to Hearthstone Logs/ directory. Derived from install_path if blank.
      log_path: ""

      # Automatically write required sections to log.config on daemon start.
      auto_patch_logconfig: true

    storage:
      # Path to the BadgerDB data directory.
      db_path: "~/.battlestream/profiles/main/data"

    output:
      # Enable JSON file output.
      enabled: true

      # Directory to write stat files to.
      path: "~/.battlestream/profiles/main/stats"

      # How often to flush current state to disk (milliseconds).
      write_interval_ms: 500

  # Add more profiles for additional installs:
  # wine:
  #   hearthstone:
  #     install_path: "/mnt/games/battlenet/drive_c/Program Files (x86)/Hearthstone"
  #   storage:
  #     db_path: "~/.battlestream/profiles/wine/data"
  #   output:
  #     enabled: true
  #     path: "~/.battlestream/profiles/wine/stats"
  #     write_interval_ms: 500

# Global settings shared across all profiles.
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

## Backward Compatibility

Configs written before profile support (with top-level `hearthstone`, `storage`,
and `output` keys) are automatically migrated to a profile named `"default"` on
first load. The migrated layout is written back on the next `battlestream discover`
save.

## Environment Variables

Environment variables override file values. Prefix: `BS_`, separator `_`.

| Variable | Config Key | Example |
|---|---|---|
| `BS_API_GRPC_ADDR` | `api.grpc_addr` | `0.0.0.0:50051` |
| `BS_API_REST_ADDR` | `api.rest_addr` | `0.0.0.0:8080` |
| `BS_API_API_KEY` | `api.api_key` | `s3cr3t` |
| `BS_LOGGING_LEVEL` | `logging.level` | `debug` |

Per-profile settings cannot be set via environment variables; use the config file.
