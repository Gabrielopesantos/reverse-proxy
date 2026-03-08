## Reverse proxy

Simple implementation of a configurable reverse proxy written in Go.

### Features

**Routing**
- Path-prefix routing with multiple upstreams per route
- Hot-reload of config file every 5 seconds (configurable) - atomic swap, zero downtime; watcher respects the root context and stops cleanly on shutdown

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

### Configuration reference

```yaml
server:
  address: :8080
  read_timeout: 10 # seconds

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
    middleware:
      - logger:
          stream: stdout # "stdout" | "stderr" | file path
          mode: text # "text" | "json"
      - rate_limiter:
          max_requests: 100
          time_frame_seconds: 60
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

#### Middleware fields

| Middleware | Field | Description |
|------------|-------|-------------|
| `logger` | `stream` | `stdout`, `stderr`, or a file path |
| `logger` | `mode` | `text` or `json` |
| `rate_limiter` | `max_requests` | Maximum requests allowed in the window |
| `rate_limiter` | `time_frame_seconds` | Sliding window size in seconds |
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
make run        # go run ./... using examples/config.yaml
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
