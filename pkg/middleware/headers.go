package middleware

import (
	"context"
	"net/http"
)

func init() {
	RegisterYAML(HEADERS, func() *Headers { return &Headers{} })
}

// Headers manipulates request and/or response headers.
// Rules under "request" apply before the upstream call; rules under "response"
// apply before the response is written back to the client.
type Headers struct {
	Request  HeaderRules `yaml:"request"`
	Response HeaderRules `yaml:"response"`
}

// HeaderRules describes the header mutations for one direction (request or response).
type HeaderRules struct {
	// Set overwrites the header value (or creates it if absent).
	Set map[string]string `yaml:"set"`
	// Add appends a value to the header (multi-value safe).
	Add map[string]string `yaml:"add"`
	// Remove deletes the header entirely.
	Remove []string `yaml:"remove"`
}

func (h *Headers) Init(_ context.Context) error { return nil }

func (h *Headers) Exec(next http.Handler) http.Handler {
	hasReqRules := len(h.Request.Set)+len(h.Request.Add)+len(h.Request.Remove) > 0
	hasRespRules := len(h.Response.Set)+len(h.Response.Add)+len(h.Response.Remove) > 0

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip Clone (full request + header copy) when there's nothing to mutate.
		req := r
		if hasReqRules {
			req = r.Clone(r.Context())
			applyHeaderRules(req.Header, h.Request)
		}

		if hasRespRules {
			next.ServeHTTP(&headerModifyWriter{ResponseWriter: w, rules: h.Response}, req)
		} else {
			next.ServeHTTP(w, req)
		}
	})
}

func (h *Headers) Close() error { return nil }

func applyHeaderRules(hdr http.Header, rules HeaderRules) {
	for k, v := range rules.Set {
		hdr.Set(k, v)
	}
	for k, v := range rules.Add {
		hdr.Add(k, v)
	}
	for _, k := range rules.Remove {
		hdr.Del(k)
	}
}

// headerModifyWriter intercepts WriteHeader/Write to inject response-header
// mutations before the status line is flushed.
type headerModifyWriter struct {
	http.ResponseWriter
	rules   HeaderRules
	applied bool
}

func (h *headerModifyWriter) Unwrap() http.ResponseWriter { return h.ResponseWriter }

func (h *headerModifyWriter) applyOnce() {
	if h.applied {
		return
	}
	h.applied = true
	applyHeaderRules(h.Header(), h.rules)
}

func (h *headerModifyWriter) WriteHeader(code int) {
	h.applyOnce()
	h.ResponseWriter.WriteHeader(code)
}

func (h *headerModifyWriter) Write(b []byte) (int, error) {
	h.applyOnce()
	return h.ResponseWriter.Write(b)
}
