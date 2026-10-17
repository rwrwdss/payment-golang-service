// Package payments описывает платёж и правила его создания.
package payments

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status — этап, на котором находится платёж.
type Status string

const (
	StatusNew     Status = "new"
	StatusWaiting Status = "waiting"
	StatusSuccess Status = "success"
	StatusDenied  Status = "denied"
)

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

// Payment — один платёж клиента. Суммы хранятся в минимальных единицах (копейки, центы).
type PaymentMethod string

const (
	MethodApplePay  PaymentMethod = "applepay"
	MethodGooglePay PaymentMethod = "googlepay"
	MethodCard      PaymentMethod = "card"
	MethodSBP       PaymentMethod = "sbp"
)

// IsValid сообщает, поддерживается ли этот способ оплаты.
func (p PaymentMethod) IsValid() bool {
	switch p {
	case MethodApplePay, MethodGooglePay, MethodCard, MethodSBP:
		return true
	}
	return false
}

// Ошибки, которые возвращает NewPayment при неверных данных.
var (
	ErrInvalidAmount           = errors.New("payments: amount must be positive")
	ErrInvalidCurrency         = errors.New("payments: currency is not supported")
	ErrPaymentMethodNotAllowed = errors.New("payments: payment method not allowed")
)

type Payment struct {
	ID        uuid.UUID
	Amount    int64
	Currency  Currency
	Status    Status
	CreatedAt time.Time
	method    PaymentMethod // Приватное: задается только в конструкторе
}

func NewPayment(amount int64, currency Currency, paymentMethod PaymentMethod) (*Payment, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	if !currency.IsValid() {
		return nil, ErrInvalidCurrency
	}

	if !paymentMethod.IsValid() {
		return nil, ErrPaymentMethodNotAllowed
	}

	return &Payment{
		ID:        uuid.New(),
		Amount:    amount,
		Currency:  currency,
		Status:    StatusNew,
		CreatedAt: time.Now(),
		method:    paymentMethod,
	}, nil
}

// Method возвращает способ оплаты платежа.
func (p *Payment) Method() PaymentMethod { return p.method }
