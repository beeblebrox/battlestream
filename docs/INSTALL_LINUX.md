# Linux Installation

Hearthstone on Linux runs via Wine or Proton (Steam). battlestream supports both.

## Wine

### Log paths

| File | Path |
|---|---|
| Logs directory | `~/.wine/drive_c/Program Files (x86)/Hearthstone/Logs/` |
| log.config | `~/.wine/drive_c/users/<username>/AppData/Local/Blizzard/Hearthstone/log.config` |

### Setup

```sh
battlestream discover
# If auto-detection fails, enter: ~/.wine/drive_c/Program Files (x86)/Hearthstone
battlestream daemon
```

## Proton / Steam (Native)

Hearthstone is not on Steam natively, but may be run via Lutris with Proton.

Proton prefix is typically at:
```
~/.local/share/Steam/steamapps/compatdata/<appid>/pfx/
```

For Hearthstone via Lutris/custom Proton, use `battlestream discover` and enter the prefix path manually.

### log.config location (Proton)
```
~/.local/share/Steam/steamapps/compatdata/<appid>/pfx/drive_c/users/steamuser/AppData/Local/Blizzard/Hearthstone/log.config
```

## Flatpak (Battle.net via Bottles or Lutris)

If using Flatpak, the prefix is typically under `~/.var/app/`. Run:

```sh
battlestream discover
```

And enter the path to the Hearthstone game directory when prompted.

## Systemd Service

Create `/etc/systemd/system/battlestream.service`:

```ini
[Unit]
Description=Battlestream - Hearthstone BG tracker
After=network.target

[Service]
Type=simple
User=%i
ExecStart=/usr/local/bin/battlestream daemon
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now battlestream
journalctl -fu battlestream
```

Or as a user service (`~/.config/systemd/user/battlestream.service`):

```sh
systemctl --user enable --now battlestream
```
