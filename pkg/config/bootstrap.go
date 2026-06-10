package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	// TODO: This shouldn't be the default config
	defaultConfigPath = "examples/config.yaml"
	defaultListenAddr = ":8080"
	defaultAdminAddr  = ":9090"

	defaultWatchInterval = 5 * time.Second
	defaultReadTimeout   = 10 * time.Second

	defaultLogFormat = "text"
	defaultLogOutput = "stdout"
	defaultLogColor  = "auto"

	envConfigPath     = "RP_CONFIG_PATH"
	envReloadInterval = "RP_CONFIG_RELOAD_INTERVAL"
	envListenAddr     = "RP_LISTEN_ADDR"
	envAdminAddr      = "RP_ADMIN_ADDR"
	envReadTimeout    = "RP_READ_TIMEOUT"
	envLogLevel       = "RP_LOG_LEVEL"
	envLogFormat      = "RP_LOG_FORMAT"
	envLogOutput      = "RP_LOG_OUTPUT"
	envLogColor       = "RP_LOG_COLOR"
	envTLSCert        = "RP_TLS_CERT"
	envTLSKey         = "RP_TLS_KEY"
)

type BootstrapConfig struct {
	ConfigPath     string
	ReloadInterval time.Duration

	ListenAddr string
	AdminAddr  string // separate listener for /healthz and /metrics; "" disables

	ReadTimeout time.Duration

	LogLevel  slog.Level
	LogFormat string // "text" | "json"
	LogOutput string // "stdout" | "stderr" | file path
	LogColor  string // "auto" | "always" | "never"

	TLSCertFile string // path to TLS certificate; enables HTTPS when non-empty
	TLSKeyFile  string // path to TLS private key; must be set together with TLSCertFile
}

func DefaultBootstrapConfig() BootstrapConfig {
	return BootstrapConfig{
		ConfigPath:     defaultConfigPath,
		ReloadInterval: defaultWatchInterval,
		ListenAddr:     defaultListenAddr,
		AdminAddr:      defaultAdminAddr,
		ReadTimeout:    defaultReadTimeout,
		LogLevel:       slog.LevelInfo,
		LogFormat:      defaultLogFormat,
		LogOutput:      defaultLogOutput,
		LogColor:       defaultLogColor,
	}
}

// LoadBootstrapConfig keeps backward compatibility and reads from process args/env.
func LoadBootstrapConfig() (*BootstrapConfig, error) {
	return LoadBootstrap(os.Args[1:], os.Environ())
}

// LoadBootstrap parses bootstrap configuration from defaults, then env,
// then args/flags (args have highest precedence).
func LoadBootstrap(args []string, environ []string) (*BootstrapConfig, error) {
	cfg := DefaultBootstrapConfig()

	// Env overrides
	env := parseEnv(environ)

	if v := envGet(env, envConfigPath); v != "" {
		cfg.ConfigPath = v
	}
	if v := envGet(env, envReloadInterval); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid duration %q: %w", envReloadInterval, v, err)
		}
		cfg.ReloadInterval = d
	}
	if v := envGet(env, envListenAddr); v != "" {
		cfg.ListenAddr = v
	}
	if v, ok := env[envAdminAddr]; ok {
		// Allow explicit empty value to disable the admin listener.
		cfg.AdminAddr = strings.TrimSpace(v)
	}
	if v := envGet(env, envReadTimeout); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid duration %q: %w", envReadTimeout, v, err)
		}
		cfg.ReadTimeout = d
	}
	if v := envGet(env, envLogLevel); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return nil, fmt.Errorf("%s: %w", envLogLevel, err)
		}
	}
	if v := envGet(env, envLogFormat); v != "" {
		cfg.LogFormat = strings.ToLower(v) // validated below
	}
	if v := envGet(env, envLogOutput); v != "" {
		cfg.LogOutput = v
	}
	if v := envGet(env, envLogColor); v != "" {
		cfg.LogColor = strings.ToLower(v) // validated below
	}
	if v := envGet(env, envTLSCert); v != "" {
		cfg.TLSCertFile = v
	}
	if v := envGet(env, envTLSKey); v != "" {
		cfg.TLSKeyFile = v
	}

	// Args/flags overrides
	fs := flag.NewFlagSet("reverse-proxy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config-path", cfg.ConfigPath, "Path to runtime YAML config file")
	reloadInterval := fs.Duration("config-reload-interval", cfg.ReloadInterval, fmt.Sprintf("Config watch/reload interval (e.g. %ds)", defaultWatchInterval))
	listenAddr := fs.String("listen-addr", cfg.ListenAddr, "HTTP listen address")
	adminAddr := fs.String("admin-addr", cfg.AdminAddr, "Admin listener for /healthz and /metrics; empty disables")
	readTimeout := fs.Duration("read-timeout", cfg.ReadTimeout, fmt.Sprintf("HTTP server read timeout (e.g. %ds)", defaultReadTimeout))
	fs.TextVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level: debug|info|warn|error")
	logFormat := fs.String("log-format", cfg.LogFormat, "Log format: text|json")
	logOutput := fs.String("log-output", cfg.LogOutput, "Log output: stdout|stderr|/path/to/file")
	logColor := fs.String("log-color", cfg.LogColor, "Log color mode: auto|always|never")
	tlsCert := fs.String("tls-cert", cfg.TLSCertFile, "Path to TLS certificate file (enables HTTPS + HTTP/2)")
	tlsKey := fs.String("tls-key", cfg.TLSKeyFile, "Path to TLS private key file")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg.ConfigPath = strings.TrimSpace(*configPath)
	cfg.ReloadInterval = *reloadInterval
	cfg.ListenAddr = strings.TrimSpace(*listenAddr)
	cfg.AdminAddr = strings.TrimSpace(*adminAddr)
	cfg.ReadTimeout = *readTimeout
	cfg.LogFormat = strings.ToLower(strings.TrimSpace(*logFormat))
	cfg.LogOutput = strings.TrimSpace(*logOutput)
	cfg.LogColor = strings.ToLower(strings.TrimSpace(*logColor))
	cfg.TLSCertFile = strings.TrimSpace(*tlsCert)
	cfg.TLSKeyFile = strings.TrimSpace(*tlsKey)

	if err := validateLogFormat(cfg.LogFormat); err != nil {
		return nil, fmt.Errorf("log-format: %w", err)
	}
	if err := validateLogColor(cfg.LogColor); err != nil {
		return nil, fmt.Errorf("log-color: %w", err)
	}

	if cfg.ConfigPath == "" {
		return nil, fmt.Errorf("config-path cannot be empty")
	}
	if cfg.ReloadInterval <= 0 {
		return nil, fmt.Errorf("config-reload-interval must be > 0")
	}
	if cfg.ListenAddr == "" {
		return nil, fmt.Errorf("listen-addr cannot be empty")
	}
	if cfg.ReadTimeout <= 0 {
		return nil, fmt.Errorf("read-timeout must be > 0")
	}
	if cfg.LogOutput == "" {
		return nil, fmt.Errorf("log-output cannot be empty")
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return nil, fmt.Errorf("tls-cert and tls-key must both be set or both be empty")
	}

	return &cfg, nil
}

func parseEnv(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, kv := range environ {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		out[kv[:i]] = kv[i+1:]
	}
	return out
}

func envGet(env map[string]string, key string) string {
	return strings.TrimSpace(env[key])
}

func validateLogFormat(raw string) error {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("invalid format %q (expected text|json)", raw)
	}
}

func validateLogColor(raw string) error {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "auto", "always", "never":
		return nil
	default:
		return fmt.Errorf("invalid color mode %q (expected auto|always|never)", raw)
	}
}
