package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

const defaultMaxBodyBytes = 65536

// WAFConfig implements a Web Application Firewall middleware that inspects
// requests for common attack patterns.
type WAFConfig struct {
	Mode         string   `json:"mode"`           // "block" (default) | "log"
	Rules        []string `json:"rules"`          // which built-in rule sets to enable; empty = all
	MaxBodyBytes int64    `json:"max_body_bytes"` // max bytes of body to inspect; default 65536

	compiled []*compiledRule
}

type compiledRule struct {
	name string
	re   *regexp.Regexp
}

var builtinRuleSets = map[string]string{
	"sql_injection":     `(?i)\b(select|union|insert|update|delete|drop|alter|exec|execute|truncate)\b|(?i)('\s*(or|and)\s+'?\d|--[ \t]*$|;\s*--)`,
	"xss":               `(?i)<\s*script|(?i)(javascript|vbscript|data)\s*:|(?i)\bon\w+\s*=`,
	"path_traversal":    `(?i)(\.\.[\\/]|%2e%2e[\\/\%]|\.\.%2f|%2e%2e%2f)`,
	"command_injection": "(?i)[|;&`]|\\$\\([^)]+\\)",
}

func (w *WAFConfig) Init(_ context.Context) error {
	if w.MaxBodyBytes == 0 {
		w.MaxBodyBytes = defaultMaxBodyBytes
	}
	if w.Mode == "" {
		w.Mode = "block"
	}

	names := w.Rules
	if len(names) == 0 {
		for name := range builtinRuleSets {
			names = append(names, name)
		}
	}

	for _, name := range names {
		pat, ok := builtinRuleSets[name]
		if !ok {
			return fmt.Errorf("waf: unknown rule %q", name)
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return err
		}
		w.compiled = append(w.compiled, &compiledRule{name: name, re: re})
	}
	return nil
}

func (w *WAFConfig) Exec(next http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if matched, rule, field := w.scan(r); matched {
			slog.Default().Warn("WAF: suspicious request", "rule", rule, "field", field, "path", r.URL.Path, "ip", clientIP(r))
			if w.Mode == "block" {
				http.Error(rw, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(rw, r)
	}
}

func (w *WAFConfig) scan(r *http.Request) (matched bool, ruleName, field string) {
	// 1. URL path + query
	target := r.URL.RawPath + "?" + r.URL.RawQuery
	if target == "?" {
		target = r.URL.Path + "?" + r.URL.RawQuery
	}
	if rule := w.matchAny(target); rule != "" {
		return true, rule, "url"
	}

	// 2. Selected headers
	for _, hdr := range []string{"User-Agent", "Referer", "Cookie"} {
		if val := r.Header.Get(hdr); val != "" {
			if rule := w.matchAny(val); rule != "" {
				return true, rule, hdr
			}
		}
	}

	// 3. Request body (only for text-like content types)
	ct := r.Header.Get("Content-Type")
	if r.Body != nil && isInspectableContentType(ct) {
		body, err := io.ReadAll(io.LimitReader(r.Body, w.MaxBodyBytes))
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		if err == nil {
			if rule := w.matchAny(string(body)); rule != "" {
				return true, rule, "body"
			}
		}
	}

	return false, "", ""
}

func (w *WAFConfig) matchAny(s string) string {
	for _, cr := range w.compiled {
		if cr.re.MatchString(s) {
			return cr.name
		}
	}
	return ""
}

func isInspectableContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "application/x-www-form-urlencoded") ||
		strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "text/")
}
