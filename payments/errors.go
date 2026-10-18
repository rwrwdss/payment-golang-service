package payments

import "errors"

// Ошибки, которые возвращает NewPayment при неверных данных.
var (
	ErrInvalidAmount   = errors.New("payments: amount must be positive")
	ErrInvalidCurrency = errors.New("payments: currency is not supported")
	ErrInvalidMethod   = errors.New("payments: payment method is not supported")
)
