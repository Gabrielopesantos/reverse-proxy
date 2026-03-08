## Reverse proxy

Simple implementation of a configurable reverse proxy written in Go.

### Features

**Routing**
- Path-prefix routing with multiple upstreams per route
- Hot-reload of runtime config file every 5 seconds (bootstrap-configurable) with atomic swap and zero downtime

**Load balancing**
- `random` (default), `round_robin`, `weighted_round_robin`, `least_connections`, `ip_hash`
- Unhealthy hosts are automatically skipped

**Health checking**
- TCP dial per upstream on a configurable interval (default: 5 s)
- Prometheus gauge exported per host

**Forwarded headers**
- `X-Forwarded-For`, `X-Real-IP`, and `X-Forwarded-Proto` injected on every proxied request

**Observability**
- `/healthz` liveness endpoint
- `/metrics` Prometheus endpoint
- Per-route request counter and latency histogram via the `prometheus` middleware

**Middleware** (ordered, per-route)
- `logger` - structured request logging
- `rate_limiter` - sliding-window rate limit by client IP
- `basic_auth` - bcrypt-hashed credentials loaded from a file
- `cache_control` - LRU-backed response cache with TTL
- `prometheus` - per-route Prometheus metrics
- `waf` - Web Application Firewall; blocks or logs requests matching built-in attack signatures

**Graceful shutdown**
- SIGINT/SIGTERM cancels the root context, stopping the config watcher and draining the HTTP server within 5 s

---

## Configuration model

This project uses **two configuration lifecycles**:

1. **Bootstrap config (restart required)**  
   Process wiring and startup behavior. Changes require restarting the process.

2. **Runtime config (hot-reloadable YAML)**  
   Routing and middleware behavior loaded from `examples/config.yaml` (or configured path). Changes are applied without restart.

### Restart-required (bootstrap) settings

These should be provided via CLI flags / env vars (recommended) and are **not** part of runtime route YAML:

- Runtime config file path (default: `examples/config.yaml`)
- Runtime config reload interval (default: `5s`)
- Server listen address (default: `:8080`)
- Server read timeout (default: `10s`)
- Global logger level (`debug|info|warn|error`)
- Global logger format (`text|json`)
- Global logger output (`stdout|stderr|/path/to/file`)
- Global logger color (`auto|true|false`)

Suggested env vars:
- `RP_CONFIG_PATH`
- `RP_CONFIG_RELOAD_INTERVAL`
- `RP_LISTEN_ADDR`
- `RP_LOG_LEVEL`
- `RP_LOG_FORMAT`
- `RP_LOG_OUTPUT`
- `RP_LOG_COLOR`


### Hot-reloadable (runtime YAML) settings

These should be provided via the YAML runtime config:

- `routes`
- route upstreams / balancer strategy / weights
- route health-check settings
- per-route middleware list and middleware-specific fields

---

## Runtime YAML reference

```yaml
routes:
  /api:
    upstreams:
      - "http://localhost:8081"
      - "http://localhost:8082"
    lb_strategy: round_robin  # "random" (default) | "round_robin" | "weighted_round_robin" | "least_connections" | "ip_hash"
    weights: # only for weighted_round_robin; omitted hosts default to weight 1
      "http://localhost:8081": 3
      "http://localhost:8082": 1
    healthcheck_interval_seconds: 5 # default: 5
    healthcheck_path: / # optional, defaults to "/"
    middleware:
      - logger:
          stream: stdout # "stdout" | "stderr" | file path
          mode: text # "text" | "json"
      - rate_limiter:
          max_requests: 100
          time_frame_seconds: 60
          stale_client_ttl_seconds: 300
          trust_proxy_headers: false
          proxy_header_max_forwards: 5
      - basic_auth:
          file: ./examples/basic_auth # lines: "user:bcrypt_hash"
      - cache_control:
          duration: 30s # Go duration string
          max_items: 200
      - prometheus:
          route: /api # label used in metrics
      - waf:
          mode: block # "block" (default) | "log"
          rules: # omit to enable all; or list a subset:
            - sql_injection
            - xss
            - path_traversal
            - command_injection
          max_body_bytes: 65536 # bytes of request body to inspect
```

For a working example see [`examples/config.yaml`](./examples/config.yaml).

---

### Middleware fields

| Middleware | Field | Description |
|------------|-------|-------------|
| `logger` | `stream` | `stdout`, `stderr`, or a file path |
| `logger` | `mode` | `text` or `json` |
| `rate_limiter` | `max_requests` | Maximum requests allowed in the window |
| `rate_limiter` | `time_frame_seconds` | Sliding window size in seconds |
| `rate_limiter` | `stale_client_ttl_seconds` | Eviction TTL for inactive client buckets |
| `rate_limiter` | `trust_proxy_headers` | If `true`, derive client IP from `X-Forwarded-For`/`X-Real-IP` |
| `rate_limiter` | `proxy_header_max_forwards` | Max XFF entries to inspect |
| `basic_auth` | `file` | Path to credentials file (`user:bcrypt_hash` per line) |
| `cache_control` | `duration` | Response TTL as a Go duration string (e.g. `30s`, `5m`) |
| `cache_control` | `max_items` | Maximum number of cached responses (LRU eviction) |
| `prometheus` | `route` | Route label attached to all emitted metrics |
| `waf` | `mode` | `block` to reject with 403, or `log` to warn and pass through |
| `waf` | `rules` | Rule sets to enable: `sql_injection`, `xss`, `path_traversal`, `command_injection`; omit for all |
| `waf` | `max_body_bytes` | Max request body bytes to inspect (default: 65536) |

---

### Endpoints

| Endpoint | Description |
|----------|-------------|
| `/*` | Proxied to upstream based on longest matching path prefix |
| `/healthz` | Returns `200 ok` — use as a liveness probe |
| `/metrics` | Prometheus metrics |

---

### Prometheus metrics

| Metric | Type | Labels |
|--------|------|--------|
| `proxy_requests_total` | Counter | `route`, `method`, `status` |
| `proxy_request_duration_seconds` | Histogram | `route`, `method` |
| `proxy_upstream_healthy` | Gauge | `host` |

---

### Running

```bash
make run        # go run ./... using configured bootstrap/runtime settings
make fmt        # gofmt all Go files
go test ./...
go test -race ./...
```

---

### References

- [Traefik](https://doc.traefik.io/traefik/)
- [OpenFaas Watchdog](https://github.com/openfaas/of-watchdog)
- [Caddy](https://caddyserver.com/)
- [Reproxy](https://github.com/umputun/reproxy)

### License

MIT
