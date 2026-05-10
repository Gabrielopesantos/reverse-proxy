package middleware

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Factory builds a Middleware from its YAML-encoded configuration bytes.
type Factory func(ctx context.Context, enc []byte) (Middleware, error)

var globalRegistry = map[MiddlewareType]Factory{}

// Register associates a factory with a middleware type.
// Middleware implementations call this from init() to self-register.
func Register(typ MiddlewareType, f Factory) {
	globalRegistry[typ] = f
}

// RegisterYAML is a convenience helper for middleware whose config is
// deserialized from YAML. newFn must return a pointer implementing Middleware.
func RegisterYAML[T Middleware](typ MiddlewareType, newFn func() T) {
	Register(typ, func(ctx context.Context, enc []byte) (Middleware, error) {
		cfg := newFn()
		if err := yaml.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		return cfg, cfg.Init(ctx)
	})
}

// Build instantiates and initialises a Middleware of the given type.
func Build(typ MiddlewareType, ctx context.Context, enc []byte) (Middleware, error) {
	f, ok := globalRegistry[typ]
	if !ok {
		return nil, fmt.Errorf("unknown middleware type: %s", typ)
	}
	return f(ctx, enc)
}
