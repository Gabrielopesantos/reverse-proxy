## Reverse proxy

### About the project
Simple implementation of a configurable reverse proxy

### Roadmap (ideas)

- [x] Protocol support for HTTP
- [ ] Web Application Firewall
- [x] Load Balancing
- [ ] Middleware
    - [ ] Caching
    - [x] Rate limiting

Others:
- [x] Configurable middleware with JSON/YAML
- [x] Hotreloading of config file

### Config file template

```yaml
"first.localhost:8080":
  destination: "127.0.0.1:8081"
  middleware: []

"second.localhost:8080":
  destination: "127.0.0.1:8082"
  middleware: []
```

For a more complete example, check the following [config](./examples/config.yaml).

### References
- [Traefik](https://doc.traefik.io/traefik/)
- [OpenFaas Watchdog](https://github.com/openfaas/of-watchdog)
- [Caddy](https://caddyserver.com/)

### LICENSE
MIT
