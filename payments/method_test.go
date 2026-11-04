package payments

import "testing"

func TestMethodIsValid(t *testing.T) {
	tests := []struct {
		method Method
		want   bool
	}{
		{MethodCard, true},
		{MethodSBP, true},
		{"applepay", false},
		{"cash", false},
		{"CARD", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			if got := tt.method.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
