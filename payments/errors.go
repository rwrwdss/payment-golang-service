package payments

import "errors"

// Ошибки пакета payments. Сравнивайте их через errors.Is, а не через ==.
var (
	ErrInvalidAmount      = errors.New("payments: amount must be positive")
	ErrInvalidCurrency    = errors.New("payments: currency is not supported")
	ErrInvalidMethod      = errors.New("payments: payment method is not supported")
	ErrInvalidDescription = errors.New("payments: description is too long")
	ErrInvalidTransition  = errors.New("payments: invalid status transition")
	ErrMissingProviderID  = errors.New("payments: provider payment id is empty")
	ErrNotFound           = errors.New("payments: payment not found")
	ErrPaymentRejected    = errors.New("payments: provider rejected the payment")
)
