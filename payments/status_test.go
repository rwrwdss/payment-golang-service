package payments

import "testing"

func TestStatusIsFinal(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusNew, false},
		{StatusWaiting, false},
		{StatusSuccess, true},
		{StatusDenied, true},
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
		{StatusNew, StatusWaiting, true},
		{StatusWaiting, StatusSuccess, true},
		{StatusWaiting, StatusDenied, true},
		{StatusNew, StatusSuccess, false},
		{StatusNew, StatusDenied, false},
		{StatusWaiting, StatusNew, false},
		{StatusSuccess, StatusDenied, false},
		{StatusDenied, StatusSuccess, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
				t.Errorf("CanTransitionTo() = %v, want %v", got, tt.want)
			}
		})
	}
}
