---
name: bs-check
description: "Check battlestream state via gRPC - current game, stats, or game list"
disable-model-invocation: true
allowed-tools: Bash(grpcurl *)
---

# bs-check

Query battlestream state via gRPC.

## Usage

`/bs-check` — show current game
`/bs-check game` — show current game
`/bs-check stats` — show aggregate stats
`/bs-check games` — list all games

## Commands

### game (default)
```
grpcurl -plaintext 127.0.0.1:50051 battlestream.v1.BattlestreamService/GetCurrentGame
```

### stats
```
grpcurl -plaintext 127.0.0.1:50051 battlestream.v1.BattlestreamService/GetAggregate
```

### games
```
grpcurl -plaintext 127.0.0.1:50051 battlestream.v1.BattlestreamService/ListGames
```

Run the appropriate grpcurl command based on the argument. If no argument, default to `game`.
