# Windows Installation

## Prerequisites

- Windows 10/11
- Hearthstone installed via Battle.net
- Git (for cloning the repo)

## Steps

### 1. Install Go

Install Go 1.24 or later. The easiest method is winget:

```powershell
winget install GoLang.Go
```

Or download the MSI installer from https://go.dev/dl/ and run it. The installer adds Go to your PATH automatically.

Verify the install:
```powershell
go version
# should print go1.24 or later
```

### 2. Build battlestream

```powershell
git clone https://github.com/beeblebrox/battlestream
cd battlestream
go build -o battlestream.exe ./cmd/battlestream
```

### 3. Run discovery

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

### 4. Start the daemon

```powershell
.\battlestream.exe daemon
```

### 5. Run as a Windows Service (optional)

Use NSSM (Non-Sucking Service Manager):

```powershell
nssm install battlestream "C:\path\to\battlestream.exe" daemon
nssm set battlestream AppDirectory "C:\path\to\"
nssm start battlestream
```

### 6. Verify

```powershell
curl http://localhost:8080/v1/health
```

## Troubleshooting

- **Log files not found**: Run `battlestream discover` and enter the path manually.
- **log.config not patched**: Ensure Hearthstone is not running when starting the daemon.
- **Port conflicts**: Edit `~/.battlestream/config.yaml` to change `api.grpc_addr` and `api.rest_addr`.
