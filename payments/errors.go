package payments

import "errors"

// Ошибки пакета payments. Сравнивайте их через errors.Is, а не через ==.
var (
	ErrInvalidAmount     = errors.New("payments: amount must be positive")
	ErrInvalidCurrency   = errors.New("payments: currency is not supported")
	ErrInvalidMethod     = errors.New("payments: payment method is not supported")
	ErrInvalidTransition = errors.New("payments: invalid status transition")
)
