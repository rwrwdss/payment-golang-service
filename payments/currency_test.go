package payments

import "testing"

func TestCurrencyIsValid(t *testing.T) {
	tests := []struct {
		currency Currency
		want     bool
	}{
		{CurrencyRUB, true},
		{CurrencyUSD, true},
		{CurrencyEUR, true},
		{"BTC", false},
		{"rub", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.currency), func(t *testing.T) {
			if got := tt.currency.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
