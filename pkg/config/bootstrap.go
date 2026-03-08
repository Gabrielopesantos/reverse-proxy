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
	DefaultWatchInterval = 5 * time.Second
	DefaultListenAddr    = ":8080"
	DefaultReadTimeout   = 10 * time.Second
	DefaultLogFormat     = "text"
	DefaultLogOutput     = "stdout"
	DefaultLogColor      = "auto"

	EnvConfigPath     = "RP_CONFIG_PATH"
	EnvReloadInterval = "RP_CONFIG_RELOAD_INTERVAL"
	EnvListenAddr     = "RP_LISTEN_ADDR"
	EnvReadTimeout    = "RP_READ_TIMEOUT"
	EnvLogLevel       = "RP_LOG_LEVEL"
	EnvLogFormat      = "RP_LOG_FORMAT"
	EnvLogOutput      = "RP_LOG_OUTPUT"
	EnvLogColor       = "RP_LOG_COLOR"
)

type BootstrapConfig struct {
	ConfigPath     string
	ReloadInterval time.Duration
	ListenAddr     string
	ReadTimeout    time.Duration
	LogLevel       slog.Level
	LogFormat      string // "text" | "json"
	LogOutput      string // "stdout" | "stderr" | file path
	LogColor       string // "auto" | "always" | "never"
}

func DefaultBootstrapConfig() BootstrapConfig {
	return BootstrapConfig{
		ConfigPath:     DefaultPath,
		ReloadInterval: DefaultWatchInterval,
		ListenAddr:     DefaultListenAddr,
		ReadTimeout:    DefaultReadTimeout,
		LogLevel:       slog.LevelInfo,
		LogFormat:      DefaultLogFormat,
		LogOutput:      DefaultLogOutput,
		LogColor:       DefaultLogColor,
	}
}

// LoadBootstrapConfig keeps backward compatibility and reads from process args/env.
func LoadBootstrapConfig() (BootstrapConfig, error) {
	return LoadBootstrap(os.Args[1:], os.Environ())
}

// LoadBootstrap parses bootstrap configuration from defaults, then env,
// then args/flags (args have highest precedence).
func LoadBootstrap(args []string, environ []string) (BootstrapConfig, error) {
	cfg := DefaultBootstrapConfig()

	// Env overrides
	env := parseEnv(environ)

	if v := envGet(env, EnvConfigPath); v != "" {
		cfg.ConfigPath = v
	}
	if v := envGet(env, EnvReloadInterval); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return BootstrapConfig{}, fmt.Errorf("%s: invalid duration %q: %w", EnvReloadInterval, v, err)
		}
		cfg.ReloadInterval = d
	}
	if v := envGet(env, EnvListenAddr); v != "" {
		cfg.ListenAddr = v
	}
	if v := envGet(env, EnvReadTimeout); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return BootstrapConfig{}, fmt.Errorf("%s: invalid duration %q: %w", EnvReadTimeout, v, err)
		}
		cfg.ReadTimeout = d
	}
	if v := envGet(env, EnvLogLevel); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return BootstrapConfig{}, fmt.Errorf("%s: %w", EnvLogLevel, err)
		}
	}
	if v := envGet(env, EnvLogFormat); v != "" {
		cfg.LogFormat = strings.ToLower(v) // validated below
	}
	if v := envGet(env, EnvLogOutput); v != "" {
		cfg.LogOutput = v
	}
	if v := envGet(env, EnvLogColor); v != "" {
		cfg.LogColor = strings.ToLower(v) // validated below
	}

	// Args/flags overrides
	fs := flag.NewFlagSet("reverse-proxy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config-path", cfg.ConfigPath, "Path to runtime YAML config file")
	reloadInterval := fs.Duration("config-reload-interval", cfg.ReloadInterval, "Config watch/reload interval (e.g. 5s)")
	listenAddr := fs.String("listen-addr", cfg.ListenAddr, "HTTP listen address")
	readTimeout := fs.Duration("read-timeout", cfg.ReadTimeout, "HTTP server read timeout (e.g. 10s)")
	fs.TextVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level: debug|info|warn|error")
	logFormat := fs.String("log-format", cfg.LogFormat, "Log format: text|json")
	logOutput := fs.String("log-output", cfg.LogOutput, "Log output: stdout|stderr|/path/to/file")
	logColor := fs.String("log-color", cfg.LogColor, "Log color mode: auto|always|never")

	if err := fs.Parse(args); err != nil {
		return BootstrapConfig{}, err
	}

	cfg.ConfigPath = strings.TrimSpace(*configPath)
	cfg.ReloadInterval = *reloadInterval
	cfg.ListenAddr = strings.TrimSpace(*listenAddr)
	cfg.ReadTimeout = *readTimeout
	cfg.LogFormat = strings.ToLower(strings.TrimSpace(*logFormat))
	cfg.LogOutput = strings.TrimSpace(*logOutput)
	cfg.LogColor = strings.ToLower(strings.TrimSpace(*logColor))

	if err := validateLogFormat(cfg.LogFormat); err != nil {
		return BootstrapConfig{}, fmt.Errorf("log-format: %w", err)
	}
	if err := validateLogColor(cfg.LogColor); err != nil {
		return BootstrapConfig{}, fmt.Errorf("log-color: %w", err)
	}

	if cfg.ConfigPath == "" {
		return BootstrapConfig{}, fmt.Errorf("config-path cannot be empty")
	}
	if cfg.ReloadInterval <= 0 {
		return BootstrapConfig{}, fmt.Errorf("config-reload-interval must be > 0")
	}
	if cfg.ListenAddr == "" {
		return BootstrapConfig{}, fmt.Errorf("listen-addr cannot be empty")
	}
	if cfg.ReadTimeout <= 0 {
		return BootstrapConfig{}, fmt.Errorf("read-timeout must be > 0")
	}
	if cfg.LogOutput == "" {
		return BootstrapConfig{}, fmt.Errorf("log-output cannot be empty")
	}

	return cfg, nil
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
