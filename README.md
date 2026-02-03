# Hetzner DDNS

Dynamic DNS updater for Hetzner Cloud DNS. It periodically fetches your public IP and keeps A/AAAA records up to date in one or more zones.

## What It Does
- Fetches your public IP from a provider like `https://api.ipify.org`.
- Looks up the configured DNS zone(s) in Hetzner.
- Creates the record if missing.
- Updates the record only when the value changes.

## Features
- Single or multi-zone configuration
- A and AAAA records
- Safe RRSet handling with optional record preservation
- Configurable timeouts and retry/backoff
- Text or JSON logs
- Docker and Docker Compose support

## Requirements
- Go 1.25+
- Hetzner Cloud API token with DNS permissions

## Quick Start (Single Zone)
```bash
export HETZNER_TOKEN="your-token"
export ZONE_NAME="example.com"
export RECORDS="@,vpn:120"
go run ./cmd/ddns
```

## Multi-Zone Example
```bash
export HETZNER_TOKEN="your-token"

export ZONE_1_NAME="example.com"
export ZONE_1_RECORDS="@,vpn:120"

export ZONE_2_NAME="example.net"
export ZONE_2_RECORDS="home"
export ZONE_2_RECORD_TYPE="AAAA"
export ZONE_2_IP_PROVIDER="https://api64.ipify.org"

go run ./cmd/ddns
```

## Docker
Build:
```bash
docker build -t ddns-app .
```

Run:
```bash
docker run --rm \
  -e HETZNER_TOKEN=your-token \
  -e ZONE_NAME=example.com \
  -e RECORDS=@,vpn \
  ddns-app
```

## Docker Compose
```yaml
version: "3.9"
services:
  hetzner-ddns:
    image: ghcr.io/fyba-1337/hetzner-ddns:latest
    container_name: hetzner-ddns
    restart: unless-stopped
    environment:
      HETZNER_TOKEN: "your-token"
      ZONE_NAME: "example.com"
      RECORDS: "@,vpn"
      INTERVAL: "2m"
      LOG_LEVEL: "info"
      LOG_FORMAT: "text"
      PRESERVE_EXISTING_RECORDS: "true"
```

## Configuration

### Required
- `HETZNER_TOKEN`  
  Hetzner Cloud API token with DNS permissions.

### Zone Configuration
You can use either a single zone (`ZONE_NAME`) or multiple zones (`ZONE_<N>_NAME`).  
Mixing both will fail validation.

Single zone:
- `ZONE_NAME`  
  DNS zone name.
- `RECORDS` (default `@`)  
  CSV of record names. You can specify per-record TTL using `name:ttl` (seconds).

Multi-zone (N is any positive integer):
- `ZONE_<N>_NAME`  
  DNS zone name (required per zone).
- `ZONE_<N>_RECORDS` (default `@`)  
  CSV of record names for that zone. You can specify per-record TTL using `name:ttl` (seconds).
- `ZONE_<N>_RECORD_TYPE` (default from `RECORD_TYPE`)  
  `A` or `AAAA`.
- `ZONE_<N>_IP_PROVIDER` (default from `IP_PROVIDER`)  
  URL returning your public IP.
- `ZONE_<N>_TTL` (optional)  
  DNS TTL (seconds) for that zone, unless overridden by record.

### Common Settings
- `RECORD_TYPE` (default `A`)  
  Record type for zones without an override.
- `IP_PROVIDER` (default `https://api.ipify.org`)  
  Must return a plain text IP.
- `TTL` (optional)  
  Default DNS TTL (seconds) for all zones, unless overridden by zone or record.
- `INTERVAL` (default `5m`)  
  Go duration string. Example: `30s`, `5m`.
- `INTERVAL_SECONDS` (legacy fallback)  
  Used only if `INTERVAL` is unset.
- `HTTP_TIMEOUT` (default `10s`)  
  HTTP client timeout for IP fetch.
- `REQUEST_TIMEOUT` (default `20s`)  
  Timeout for each API operation.

### Reliability
- `RETRY_ATTEMPTS` (default `3`, range `1..10`)
- `RETRY_BASE_DELAY` (default `500ms`)
- `RETRY_MAX_DELAY` (default `5s`)

### Safety
- `PRESERVE_EXISTING_RECORDS` (default `true`)  
  When `true`, multi-value RRsets are preserved and the new IP is appended.  
  When `false`, RRsets are replaced with a single IP.

### Logging
- `LOG_LEVEL` (default `info`)  
  `debug`, `info`, `warn`, `error`.
- `LOG_FORMAT` (default `text`)  
  `text` or `json`.
- `USER_AGENT` (default `hetzner-ddns/1.0`)  
  Sent when fetching public IP.

## Example .env
```dotenv
HETZNER_TOKEN=your-token
ZONE_NAME=example.com
RECORDS=@,vpn:120
INTERVAL=2m
LOG_LEVEL=info
LOG_FORMAT=text
```

## Build
```bash
go build -o ddns-app ./cmd/ddns
```

## Run
```bash
./ddns-app
```

## Behavior Notes
- The app fetches your public IP and updates A/AAAA records at the given interval.
- If a record does not exist, it will be created.
- When `PRESERVE_EXISTING_RECORDS=true`, multi-record RRsets are not overwritten.

## Troubleshooting
**Build errors about missing DNS fields**  
Ensure you are using `hcloud-go` v2.36.0 or newer and the code references RRSet APIs.

**No updates happening**  
- Check that your token has DNS permissions.
- Verify the zone name matches exactly.
- Ensure the IP provider returns plain text.

## CI (GitHub Actions)
The repo includes a build/push workflow in `.github/workflows/build.yml` that pushes `:latest` to `ghcr.io/fyba-1337/hetzner-ddns` on push to `main`.

## License
MIT
