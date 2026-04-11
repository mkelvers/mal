package jikan

import "testing"

func TestIsAllowedWatchOrderType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "tv", input: "tv", want: true},
		{name: "movie", input: "movie", want: true},
		{name: "case and whitespace", input: "  TV  ", want: true},
		{name: "tv special", input: "tv special", want: false},
		{name: "ova", input: "ova", want: false},
		{name: "empty", input: "", want: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := isAllowedWatchOrderType(testCase.input)
			if got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestWatchOrderTypeLabel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "tv", input: "tv", want: "TV"},
		{name: "movie", input: "movie", want: "Movie"},
		{name: "trimmed passthrough", input: "  tv special  ", want: "tv special"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := watchOrderTypeLabel(testCase.input)
			if got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}
