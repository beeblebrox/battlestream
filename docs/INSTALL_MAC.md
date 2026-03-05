# macOS Installation

## Prerequisites

- macOS 12+
- Hearthstone installed via Battle.net
- Go 1.24+ (or download pre-built binary)

## Steps

### 1. Build battlestream

```sh
git clone https://github.com/beeblebrox/battlestream
cd battlestream
go build -o battlestream ./cmd/battlestream
sudo mv battlestream /usr/local/bin/
```

### 2. Run discovery

```sh
battlestream discover
```

Searches:
- `/Applications/Hearthstone`

`log.config` is patched at:
```
~/Library/Preferences/Blizzard/Hearthstone/log.config
```

### 3. Start the daemon

```sh
battlestream daemon
```

### 4. Run as a launchd service (optional)

Create `~/Library/LaunchAgents/io.fixates.battlestream.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>io.fixates.battlestream</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/battlestream</string>
    <string>daemon</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/tmp/battlestream.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/battlestream.err</string>
</dict>
</plist>
```

```sh
launchctl load ~/Library/LaunchAgents/io.fixates.battlestream.plist
launchctl start io.fixates.battlestream
```

### 5. Verify

```sh
curl http://localhost:8080/v1/health
```
