package middleware

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

func init() {
	RegisterYAML(TypeRewrite, func() *Rewrite { return &Rewrite{} })
}

// Rewrite rewrites the request path before forwarding to the upstream.
// Rules are evaluated in order; the first match wins.
type Rewrite struct {
	Rules    []RewriteRule `yaml:"rules"`
	compiled []*compiledRewriteRule
}

// RewriteRule maps an incoming path pattern to a replacement.
// The replacement may reference sub-matches using $1, $2, … notation.
type RewriteRule struct {
	Match   string `yaml:"match"`
	Replace string `yaml:"replace"`
}

type compiledRewriteRule struct {
	re      *regexp.Regexp
	replace string
}

func (rw *Rewrite) Init(_ context.Context) error {
	for _, r := range rw.Rules {
		re, err := regexp.Compile(r.Match)
		if err != nil {
			return fmt.Errorf("rewrite: invalid pattern %q: %w", r.Match, err)
		}
		rw.compiled = append(rw.compiled, &compiledRewriteRule{re: re, replace: r.Replace})
	}
	return nil
}

func (rw *Rewrite) Exec(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, rule := range rw.compiled {
			if rule.re.MatchString(r.URL.Path) {
				r2 := r.Clone(r.Context())
				r2.URL.Path = rule.re.ReplaceAllString(r.URL.Path, rule.replace)
				r2.URL.RawPath = "" // let the URL package re-derive the encoded form
				next.ServeHTTP(w, r2)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (rw *Rewrite) Close() error { return nil }
