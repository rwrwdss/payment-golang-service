// Package payments описывает платёж и правила его создания.
package payments

import (
	"time"

	"github.com/google/uuid"
)

// Payment — один платёж клиента. Суммы хранятся в минимальных единицах (копейки, центы).
type Payment struct {
	ID        uuid.UUID
	Amount    int64
	Currency  Currency
	Status    Status
	CreatedAt time.Time
	method    Method // Приватное: задается только в конструкторе
}

// NewPayment проверяет входные данные и создаёт платёж в статусе StatusNew.
func NewPayment(amount int64, currency Currency, method Method) (*Payment, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	if !currency.IsValid() {
		return nil, ErrInvalidCurrency
	}

	if !method.IsValid() {
		return nil, ErrInvalidMethod
	}

	return &Payment{
		ID:        uuid.New(),
		Amount:    amount,
		Currency:  currency,
		Status:    StatusNew,
		CreatedAt: time.Now(),
		method:    method,
	}, nil
}

// Method возвращает способ оплаты платежа.
func (p *Payment) Method() Method { return p.method }
