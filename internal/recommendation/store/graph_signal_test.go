package store

import "testing"

func TestGraphSignalSearchTerm_PrefersDomainTermOverStressTokens(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "stress coffee query", query: "coffee near mission worker=17 seq=42", want: "coffee"},
		{name: "ramen graph signal", query: "quiet ramen near mission", want: "ramen"},
		{name: "generic fallback", query: "bookstore by ferry building", want: "bookstore"},
		{name: "only routing tokens", query: "near mission worker=2 seq=99", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := graphSignalSearchTerm(tt.query); got != tt.want {
				t.Fatalf("graphSignalSearchTerm(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}
