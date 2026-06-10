package middleware

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Factory builds a Middleware from its YAML-encoded configuration bytes.
type Factory func(ctx context.Context, enc []byte) (Middleware, error)

// registration pairs a type's factory with the phase that fixes where it runs
// in the chain.
type registration struct {
	factory Factory
	phase   Phase
}

var globalRegistry = map[MiddlewareType]registration{}

// Register associates a factory and execution phase with a middleware type.
// Middleware implementations call this from init() to self-register; custom
// middleware do the same to slot into the chain at their declared phase.
func Register(typ MiddlewareType, phase Phase, f Factory) {
	globalRegistry[typ] = registration{factory: f, phase: phase}
}

// RegisterYAML is a convenience helper for middleware whose config is
// deserialized from YAML. newFn must return a pointer implementing Middleware.
func RegisterYAML[T Middleware](typ MiddlewareType, phase Phase, newFn func() T) {
	Register(typ, phase, func(ctx context.Context, enc []byte) (Middleware, error) {
		cfg := newFn()
		if err := yaml.Unmarshal(enc, cfg); err != nil {
			return nil, err
		}
		return cfg, cfg.Init(ctx)
	})
}

// Build instantiates and initialises a Middleware of the given type.
func Build(typ MiddlewareType, ctx context.Context, enc []byte) (Middleware, error) {
	r, ok := globalRegistry[typ]
	if !ok {
		return nil, fmt.Errorf("unknown middleware type: %s", typ)
	}
	return r.factory(ctx, enc)
}

// PhaseRank returns the sort rank for a middleware type: the registered phase
// for known types, or a rank past every phase for unregistered ones (which
// fail later in Build). Used to order a route's middleware chain.
func PhaseRank(typ MiddlewareType) int {
	if r, ok := globalRegistry[typ]; ok {
		return int(r.phase)
	}
	return int(phaseCount)
}
