## Reverse proxy
### About the project
Simple implementation of a configurable reverse proxy

### Roadmap (ideas)

- [x] Protocol support for HTTP
- [ ] Web Application Firewall
- [ ] Load Balancing
- [ ] Middleware
    - [ ] Caching
    - [ ] Rate limiting

Others:
- [ ] Configurable middlware with json/yaml
- [ ] Hotreloading of config file

### Config file template
```yaml
"first.localhost:8080":
  destination: "127.0.0.1:8081"
  middleware: []

"second.localhost:8080":
  destination: "127.0.0.1:8082"
  middleware: []
```

### References
- [Traefik](https://doc.traefik.io/traefik/)
- [OpenFaas Watchdog](https://github.com/openfaas/of-watchdog)
- [Caddy](https://caddyserver.com/)

### LICENSE
MIT
