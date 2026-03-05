# Windows Installation

## Prerequisites

- Windows 10/11
- Hearthstone installed via Battle.net
- Go 1.24+ (for building from source) or download a pre-built binary

## Steps

### 1. Build or download battlestream

```powershell
git clone https://github.com/fixates/battlestream
cd battlestream
go build -o battlestream.exe ./cmd/battlestream
```

### 2. Run discovery

```powershell
.\battlestream.exe discover
```

This will find your Hearthstone install and write `%USERPROFILE%\.battlestream\config.yaml`.

Hearthstone install paths searched automatically:
- `C:\Program Files (x86)\Hearthstone`
- `C:\Program Files\Hearthstone`

`log.config` is patched at:
```
%LOCALAPPDATA%\Blizzard\Hearthstone\log.config
```

### 3. Start the daemon

```powershell
.\battlestream.exe daemon
```

### 4. Run as a Windows Service (optional)

Use NSSM (Non-Sucking Service Manager):

```powershell
nssm install battlestream "C:\path\to\battlestream.exe" daemon
nssm set battlestream AppDirectory "C:\path\to\"
nssm start battlestream
```

### 5. Verify

```powershell
curl http://localhost:8080/v1/health
```

## Troubleshooting

- **Log files not found**: Run `battlestream discover` and enter the path manually.
- **log.config not patched**: Ensure Hearthstone is not running when starting the daemon.
- **Port conflicts**: Edit `~/.battlestream/config.yaml` to change `api.grpc_addr` and `api.rest_addr`.
