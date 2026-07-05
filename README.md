# homebridge-exporter

[![CI](https://github.com/hypercat-net/homebridge-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/hypercat-net/homebridge-exporter/actions/workflows/ci.yml) [![License](https://img.shields.io/github/license/hypercat-net/homebridge-exporter)](https://github.com/hypercat-net/homebridge-exporter/blob/main/LICENSE) [![Docker](https://img.shields.io/docker/v/hypercat-net/homebridge-exporter?label=docker)](https://hub.docker.com/r/hypercat-net/homebridge-exporter)

Standalone Prometheus exporter for [Homebridge](https://homebridge.io/) that polls the Config UI X REST API and exposes accessory characteristics as metrics.

Maintained by [hypercat-net](https://github.com/hypercat-net).

This is a **sidecar exporter** (not a Homebridge plugin). It runs separately from Homebridge and is well suited to Docker and Prometheus/Grafana stacks.

## Prerequisites

- Homebridge with **Config UI X** enabled
- Homebridge running in **insecure mode** (required for `GET /api/accessories`)

Enable insecure mode in your Homebridge `config.json`:

```json
{
  "bridge": {
    "name": "Homebridge",
    "username": "...",
    "port": 51826,
    "pin": "...",
    "insecure": true
  }
}
```

## Quick start (Docker)

1. Copy the example files and edit credentials and accessory IDs:

```bash
cp .env.example .env
cp config.example.yaml accessories.yaml
```

2. Discover accessory `unique_id` values:

```bash
docker compose run --rm homebridge-exporter /homebridge-exporter --list-accessories
```

Or run locally:

```bash
go run ./cmd/homebridge-exporter --list-accessories
```

3. Update `accessories.yaml` with your fridge/freezer `unique_id` values.

4. Seed the named config volume (first run only). The runtime image is distroless (no shell or `cp`), so copy with a one-off Alpine container:

```bash
docker run --rm \
  -v homebridge-exporter-config:/config \
  -v "$(pwd)/accessories.yaml:/seed/accessories.yaml:ro" \
  alpine:3.20 cp /seed/accessories.yaml /config/accessories.yaml
```

The volume is created when you first run compose (step 2 or 5). If it does not exist yet, run `docker volume create homebridge-exporter-config` first.

To update config later, re-run the same command after editing `accessories.yaml`, then `docker compose restart homebridge-exporter`.

Alternatively, with the exporter already running:

```bash
docker cp accessories.yaml homebridge-exporter:/config/accessories.yaml
docker compose restart homebridge-exporter
```

5. Start the exporter:

```bash
docker compose up -d
```

6. Verify endpoints:

- `http://localhost:9090/metrics` — Prometheus metrics
- `http://localhost:9090/health` — liveness (always 200 when running)
- `http://localhost:9090/ready` — readiness (200 after a successful poll within `2 × POLL_INTERVAL`)

## Configuration

### Environment variables

| Variable | Purpose | Default |
|---|---|---|
| `HOMEBRIDGE_URL` | Homebridge base URL | `http://homebridge:8581` |
| `HOMEBRIDGE_USERNAME` | Login username | — |
| `HOMEBRIDGE_PASSWORD` | Login password | — |
| `HOMEBRIDGE_OTP` | Optional 2FA code | empty |
| `HOMEBRIDGE_NOAUTH` | Use `/api/auth/noauth` instead of login | `false` |
| `EXPORTER_LISTEN_ADDR` | HTTP bind address | `:9090` |
| `EXPORTER_CONFIG_PATH` | Path to accessories YAML | `/config/accessories.yaml` |
| `EXPORTER_POLL_INTERVAL` | Upstream poll interval | `30s` |
| `EXPORTER_REQUEST_TIMEOUT` | Per-request timeout | `10s` |

### Accessory config (YAML)

See [`config.example.yaml`](config.example.yaml):

```yaml
accessories:
  - unique_id: "REPLACE_WITH_FRIDGE_UNIQUE_ID"
    label: fridge
    characteristics:
      - CurrentTemperature
  - unique_id: "REPLACE_WITH_FREEZER_UNIQUE_ID"
    label: freezer
    characteristics:
      - CurrentTemperature
```

- `unique_id` — stable identifier from `GET /api/accessories` (required)
- `label` — Prometheus `accessory` label (defaults to API `serviceName` if omitted)
- `characteristics` — keys from the accessory `values` map to export as gauges

## Discovering accessories

Use the built-in discovery helper:

```bash
homebridge-exporter --list-accessories
```

This prints a table of `uniqueId`, `serviceName`, `type`, and available `values` keys. You can also browse the Swagger UI at `http://<homebridge-host>:8581/swagger`.

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: homebridge
    static_configs:
      - targets: ['homebridge-exporter:9090']
```

## Metrics

### Accessory metrics

```
homebridge_characteristic_value{accessory="fridge", unique_id="...", characteristic="CurrentTemperature", service_type="TemperatureSensor", unit="celsius"} 4.2
```

Only numeric characteristics are exported as gauges.

### Exporter self-metrics

| Metric | Description |
|---|---|
| `homebridge_exporter_up` | 1 if the last poll cycle succeeded |
| `homebridge_exporter_last_scrape_timestamp_seconds` | Unix timestamp of last poll |
| `homebridge_exporter_scrape_duration_seconds` | Duration of last poll |
| `homebridge_exporter_scrape_errors_total` | Total failed poll cycles |
| `homebridge_exporter_accessory_up{unique_id, accessory}` | 1 if accessory was present in last successful poll |

## Example Grafana query

```promql
homebridge_characteristic_value{accessory=~"fridge|freezer", characteristic="CurrentTemperature"}
```

## Development

```bash
go test ./...
go run ./cmd/homebridge-exporter
```

Build the Docker image locally:

```bash
docker build -t homebridge-exporter .
```

## CI / Docker image

GitHub Actions runs `go test ./...` on every push and pull request. Pushes to
`main` (and version tags `v*`) also build and publish
[`hypercat-net/homebridge-exporter`](https://hub.docker.com/r/hypercat-net/homebridge-exporter)
to Docker Hub. A **weekly scheduled rebuild** (Sundays 04:17 UTC) refreshes
`latest` against the current `golang:1.23-alpine` base even when application
code has not changed — useful for picking up base-image CVE fixes.

Configure these [repository secrets](https://github.com/hypercat-net/homebridge-exporter/settings/secrets/actions):

| Secret | Description |
| --- | --- |
| `DOCKERHUB_USERNAME` | Docker Hub username (`hypercat-net`) |
| `DOCKERHUB_TOKEN` | Docker Hub [access token](https://hub.docker.com/settings/security) |

Tags: `latest` and `sha-<commit>` on every `main` push and weekly rebuild;
`1.0.0`, `1.0`, and `1` when you push a version tag (e.g. `v1.0.0`). Images
are published for `linux/amd64` and `linux/arm64`. A weekly workflow deletes
`sha-*` tags older than 90 days (semver and `latest` are never removed); run
[Prune Docker sha tags](https://github.com/hypercat-net/homebridge-exporter/actions/workflows/prune-docker-tags.yml)
manually with **dry run** first to preview.

Pin production to a semver or `sha-` tag; use `latest` only if you pull
regularly (or rely on the weekly rebuild) to stay on patched base layers.

Production run example (named volume for config):

```bash
docker volume create homebridge-exporter-config
docker run --rm \
  -v homebridge-exporter-config:/config \
  -v "$(pwd)/accessories.yaml:/seed/accessories.yaml:ro" \
  alpine:3.20 cp /seed/accessories.yaml /config/accessories.yaml

docker run -d --env-file .env \
  -v homebridge-exporter-config:/config \
  -p 9090:9090 \
  hypercat-net/homebridge-exporter:latest
```

## License

See [LICENSE](LICENSE).
