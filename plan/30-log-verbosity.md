# 30 — [IMPROVEMENT] Log verbosity not configurable at runtime

**Priority:** LOW
**Status:** DONE
**Area:** `cmd/battlestream/main.go`, `internal/config/`

**Resolution:** Already implemented — `setupLogging()` reads `cfg.Logging.Level`
(debug/info/warn/error) and configures slog handler. Supports `logging.level` in
config YAML and file output via `logging.file`.

## Problem

`slog.Info` calls are scattered throughout the processor (player identification, hero
entity updates, etc.). There is no way to reduce verbosity without recompiling. In
production use, the daemon generates significant log noise from routine state updates
that are only useful for debugging.

## Fix

### Step 1: Add a log level flag

In `cmd/battlestream/main.go` (or the daemon subcommand), add a `--log-level` flag:

```go
rootCmd.PersistentFlags().String("log-level", "info", "Log level: debug, info, warn, error")
```

### Step 2: Read from config / env

Support `BS_LOG_LEVEL` env var via viper (consistent with existing `BS_` prefix):

```go
viper.BindEnv("log_level", "BS_LOG_LEVEL")
```

### Step 3: Configure slog at startup

```go
level := slog.LevelInfo
switch strings.ToLower(viper.GetString("log_level")) {
case "debug": level = slog.LevelDebug
case "warn":  level = slog.LevelWarn
case "error": level = slog.LevelError
}
slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
```

### Step 4: Audit existing log levels

Review all `slog.Info` calls and downgrade routine per-event logs to `slog.Debug`.
Reserve `slog.Info` for significant state transitions (game start/end, player identified).

## Files to change

- `cmd/battlestream/main.go` — add `--log-level` flag and slog configuration
- `internal/config/` — add `log_level` to config schema
- `internal/gamestate/processor.go` — audit and downgrade routine Info to Debug

## Complexity

Low — standard slog configuration pattern.

## Verification

- `BS_LOG_LEVEL=warn ./battlestream daemon` produces no Info output during normal operation.
- `BS_LOG_LEVEL=debug` shows all per-event processing detail.
