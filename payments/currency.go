package payments

// Currency — код валюты в формате ISO 4217.
type Currency string

const (
	CurrencyRUB Currency = "RUB"
	CurrencyUSD Currency = "USD"
	CurrencyEUR Currency = "EUR"
)

// IsValid сообщает, поддерживает ли сервис эту валюту.
func (c Currency) IsValid() bool {
	switch c {
	case CurrencyRUB, CurrencyUSD, CurrencyEUR:
		return true
	}
	return false
}
