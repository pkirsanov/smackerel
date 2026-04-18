package recipe

import (
	"testing"
)

func TestFormatQuantity_FractionTable(t *testing.T) {
	cases := []struct {
		input    float64
		expected string
	}{
		{0.125, "1/8"},
		{0.25, "1/4"},
		{0.333, "1/3"},
		{0.5, "1/2"},
		{0.667, "2/3"},
		{0.75, "3/4"},
		{0.167, "1/6"},
		{0.375, "3/8"},
		{0.625, "5/8"},
		{0.875, "7/8"},
	}
	for _, tc := range cases {
		got := FormatQuantity(tc.input)
		if got != tc.expected {
			t.Errorf("FormatQuantity(%f) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestFormatQuantity_MixedNumbers(t *testing.T) {
	cases := []struct {
		input    float64
		expected string
	}{
		{1.5, "1 1/2"},
		{2.333, "2 1/3"},
		{3.25, "3 1/4"},
		{1.75, "1 3/4"},
	}
	for _, tc := range cases {
		got := FormatQuantity(tc.input)
		if got != tc.expected {
			t.Errorf("FormatQuantity(%f) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestFormatQuantity_Integers(t *testing.T) {
	cases := []struct {
		input    float64
		expected string
	}{
		{3.0, "3"},
		{1.0, "1"},
		{10.0, "10"},
		{50.0, "50"},
		{400.0, "400"},
	}
	for _, tc := range cases {
		got := FormatQuantity(tc.input)
		if got != tc.expected {
			t.Errorf("FormatQuantity(%f) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestFormatQuantity_Zero(t *testing.T) {
	got := FormatQuantity(0)
	if got != "0" {
		t.Errorf("FormatQuantity(0) = %q, want '0'", got)
	}
}
