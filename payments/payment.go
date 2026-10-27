// Package payments описывает платёж и правила его создания.
package payments

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Payment — один платёж клиента.
// Поля приватные: читаются через геттеры, статус меняется только через методы переходов.
type Payment struct {
	id        uuid.UUID // уникальный идентификатор, выдаётся при создании
	amount    int64     // сумма в минимальных единицах валюты (копейки, центы)
	currency  Currency  // валюта платежа
	method    Method    // способ оплаты, задаётся только в конструкторе
	status    Status    // текущий этап платежа
	createdAt time.Time // когда платёж создан
	updatedAt time.Time // когда статус менялся последний раз
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

	now := time.Now()

	return &Payment{
		id:        uuid.New(),
		amount:    amount,
		currency:  currency,
		method:    method,
		status:    StatusNew,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// ID возвращает уникальный идентификатор платежа.
func (p *Payment) ID() uuid.UUID { return p.id }

// Amount возвращает сумму в минимальных единицах валюты.
func (p *Payment) Amount() int64 { return p.amount }

// Currency возвращает валюту платежа.
func (p *Payment) Currency() Currency { return p.currency }

// Method возвращает способ оплаты платежа.
func (p *Payment) Method() Method { return p.method }

// Status возвращает текущий статус платежа.
func (p *Payment) Status() Status { return p.status }

// CreatedAt возвращает время создания платежа.
func (p *Payment) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt возвращает время последней смены статуса.
func (p *Payment) UpdatedAt() time.Time { return p.updatedAt }

// StartProcessing отмечает, что платёж отправлен в банк.
func (p *Payment) StartProcessing() error { return p.transitionTo(StatusWaiting) }

// Succeed отмечает, что банк подтвердил списание.
func (p *Payment) Succeed() error { return p.transitionTo(StatusSuccess) }

// Deny отмечает, что банк отклонил платёж.
func (p *Payment) Deny() error { return p.transitionTo(StatusDenied) }

func (p *Payment) transitionTo(next Status) error {
	if !p.status.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, p.status, next)
	}

	p.status = next
	p.updatedAt = time.Now()
	return nil
}
