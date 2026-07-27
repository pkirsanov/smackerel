package microtools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenMeteoGeocoderPreservesSameNameAmbiguity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.URL.Query().Get("name"); got != "Springfield" {
			t.Fatalf("name query = %q, want Springfield", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"results":[{"name":"Springfield","admin1":"Illinois","country":"United States","latitude":39.8,"longitude":-89.6},{"name":"Springfield","admin1":"Missouri","country":"United States","latitude":37.2,"longitude":-93.3}]}`)
	}))
	defer server.Close()

	provider := NewOpenMeteoGeocoder(server.Client(), server.URL)
	candidates, err := provider.Geocode(context.Background(), "Springfield")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if candidates[0].Confidence != candidates[1].Confidence {
		t.Fatalf("same-name confidence = %.2f/%.2f, want equal ambiguity", candidates[0].Confidence, candidates[1].Confidence)
	}
	if got := shapeEnvelope("open-meteo", candidates, 0.5, 5).Status; got != StatusAmbiguous {
		t.Fatalf("same-name envelope status = %q, want ambiguous", got)
	}
}
