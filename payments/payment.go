// Package payments описывает платёж и правила его создания.
package payments

import (
	"time"

	"github.com/google/uuid"
)

// Payment — один платёж клиента. Суммы хранятся в минимальных единицах (копейки, центы).
type Payment struct {
	id        uuid.UUID
	amount    int64
	currency  Currency
	status    Status
	createdAt time.Time
	updatedAt time.Time
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
		id:        uuid.New(),
		amount:    amount,
		currency:  currency,
		status:    StatusNew,
		createdAt: time.Now(),
		method:    method,
	}, nil
}

// Method возвращает способ оплаты платежа.
func (p *Payment) Method() Method { return p.method }

// Геттеры
func (p *Payment) ID() uuid.UUID        { return p.id }
func (p *Payment) Amount() int64        { return p.amount }
func (p *Payment) Currency() Currency   { return p.currency }
func (p *Payment) Status() Status       { return p.status }
func (p *Payment) CreatedAt() time.Time { return p.createdAt }
func (p *Payment) UpdatedAt() time.Time { return p.updatedAt }
