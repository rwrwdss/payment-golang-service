package payments

import "testing"

func TestStatusIsFinal(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusNew, false},
		{StatusPending, false},
		{StatusSucceeded, true},
		{StatusCanceled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsFinal(); got != tt.want {
				t.Errorf("IsFinal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusCanTransitionTo(t *testing.T) {
	tests := []struct {
		from Status
		to   Status
		want bool
	}{
		{StatusNew, StatusPending, true},
		{StatusNew, StatusCanceled, true},
		{StatusPending, StatusSucceeded, true},
		{StatusPending, StatusCanceled, true},
		{StatusNew, StatusSucceeded, false},
		{StatusPending, StatusNew, false},
		{StatusSucceeded, StatusCanceled, false},
		{StatusCanceled, StatusSucceeded, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
				t.Errorf("CanTransitionTo() = %v, want %v", got, tt.want)
			}
		})
	}
}
