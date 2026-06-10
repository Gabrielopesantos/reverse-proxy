package middleware

import "testing"

// TestPhaseRankOrder pins the outermost->innermost phase ordering across
// representative middleware types.
func TestPhaseRankOrder(t *testing.T) {
	order := []MiddlewareType{
		TypeLogger,       // observe
		TypeRateLimiter,  // guard
		TypeBasicAuth,    // authenticate
		TypeHeaders,      // shape
		TypeCacheControl, // cache
	}
	for i := 1; i < len(order); i++ {
		if PhaseRank(order[i-1]) >= PhaseRank(order[i]) {
			t.Errorf("expected %s to rank before %s, got %d >= %d",
				order[i-1], order[i], PhaseRank(order[i-1]), PhaseRank(order[i]))
		}
	}
}

// TestSamePhaseSharesRank verifies types that should share a phase do, so the
// config list order (not phase) decides their relative position.
func TestSamePhaseSharesRank(t *testing.T) {
	pairs := [][2]MiddlewareType{
		{TypeLogger, TypePrometheus},
		{TypeRateLimiter, TypeWAF},
		{TypeHeaders, TypeRewrite},
	}
	for _, p := range pairs {
		if PhaseRank(p[0]) != PhaseRank(p[1]) {
			t.Errorf("%s and %s should share a phase, got %d != %d",
				p[0], p[1], PhaseRank(p[0]), PhaseRank(p[1]))
		}
	}
}

// TestUnknownTypeSortsLast ensures an unregistered type ranks after every phase
// instead of colliding with PhaseObserve (rank 0).
func TestUnknownTypeSortsLast(t *testing.T) {
	if got, want := PhaseRank("does_not_exist"), int(phaseCount); got != want {
		t.Errorf("PhaseRank(unknown) = %d, want %d", got, want)
	}
}

// TestEveryRegisteredTypeHasValidPhase guards against a middleware registering
// with a phase outside the known range (which would misplace it in the chain).
func TestEveryRegisteredTypeHasValidPhase(t *testing.T) {
	for typ, reg := range globalRegistry {
		if reg.phase < PhaseObserve || reg.phase >= phaseCount {
			t.Errorf("middleware %q registered with out-of-range phase %d", typ, reg.phase)
		}
	}
}
