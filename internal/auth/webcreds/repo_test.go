package webcreds

import (
	"testing"
)

func TestValidateUsername(t *testing.T) {
	cases := []struct {
		name    string
		want    bool // true == valid (nil error)
		input   string
		comment string
	}{
		{name: "simple", want: true, input: "operator"},
		{name: "with_dash", want: true, input: "philip-k"},
		{name: "with_dot", want: true, input: "philip.k"},
		{name: "with_at", want: true, input: "philip@example.com"},
		{name: "max_length", want: true, input: stringRepeat("a", MaxUsernameLength)},

		{name: "empty", want: false, input: ""},
		{name: "only_space", want: false, input: "   "},
		{name: "leading_space", want: false, input: " operator"},
		{name: "trailing_space", want: false, input: "operator "},
		{name: "embedded_newline", want: false, input: "phi\nlipk"},
		{name: "embedded_tab", want: false, input: "phi\tlipk"},
		{name: "embedded_null", want: false, input: "phi\x00lipk"},
		{name: "too_long", want: false, input: stringRepeat("a", MaxUsernameLength+1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateUsername(c.input)
			if c.want && err != nil {
				t.Errorf("ValidateUsername(%q) expected nil, got %v", c.input, err)
			}
			if !c.want && err == nil {
				t.Errorf("ValidateUsername(%q) expected err, got nil", c.input)
			}
		})
	}
}

func stringRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
