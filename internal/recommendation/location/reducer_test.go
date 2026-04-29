package location

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/recommendation"
)

func TestReducerFailsClosedOnMissingOrInvalidPrecision(t *testing.T) {
	reducer := NewReducer(Config{NeighborhoodCellSystem: "h3", NeighborhoodCellLevel: 9})

	for _, tc := range []struct {
		name      string
		precision recommendation.PrecisionPolicy
	}{
		{name: "missing precision", precision: ""},
		{name: "invalid precision", precision: recommendation.PrecisionPolicy("block")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := reducer.Reduce(context.Background(), RawLocationRef{Ref: "gps:37.7749,-122.4194"}, tc.precision); err == nil {
				t.Fatal("Reduce succeeded; want fail-closed error")
			}
		})
	}
}

func TestReducerRedactsRawGPSForNeighborhoodPrecision(t *testing.T) {
	reducer := NewReducer(Config{NeighborhoodCellSystem: "h3", NeighborhoodCellLevel: 9})

	geometry, err := reducer.Reduce(context.Background(), RawLocationRef{Ref: "gps:37.7749,-122.4194"}, recommendation.PrecisionNeighborhood)
	if err != nil {
		t.Fatalf("Reduce returned error: %v", err)
	}
	if geometry.Precision != recommendation.PrecisionNeighborhood {
		t.Fatalf("Precision = %q, want neighborhood", geometry.Precision)
	}
	joined := geometry.CellID + " " + geometry.Label
	for _, raw := range []string{"37.7749", "122.4194", "gps:"} {
		if strings.Contains(joined, raw) {
			t.Fatalf("reduced geometry leaked raw location token %q in %q", raw, joined)
		}
	}
	if geometry.CellID == "" || geometry.Label == "" {
		t.Fatalf("reduced geometry missing stable cell data: %+v", geometry)
	}
}
