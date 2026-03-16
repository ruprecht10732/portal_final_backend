package timekit

import (
	"testing"
	"time"
)

func TestResolveLocationReturnsNamedLocationWhenAvailable(t *testing.T) {
	t.Parallel()

	loc := ResolveLocation("UTC")
	if loc == nil {
		t.Fatal("expected non-nil location")
	}
	if loc.String() != "UTC" {
		t.Fatalf("expected UTC location, got %q", loc.String())
	}
}

func TestResolveLocationFallsBackForInvalidTimezone(t *testing.T) {
	t.Parallel()

	loc := ResolveLocation("Invalid/Timezone")
	if loc == nil {
		t.Fatal("expected non-nil fallback location")
	}
	if loc != time.Local && loc != time.UTC {
		t.Fatalf("expected local or UTC fallback, got %q", loc.String())
	}
}
