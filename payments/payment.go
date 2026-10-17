package payments

import (
	"errors"
	"time"
)

type Status string

// Payment status pool
const (
	StatusNew     Status = "new"
	StatusWaiting Status = "waiting"
	StatusSuccess Status = "success"
	StatusDenied  Status = "denied"
)

var ErrInvalidAmount = errors.New("payments: amount must be positive")

type Payment struct {
	ID        string
	Amount    int64
	Currency  string
	Status    Status
	CreatedAt time.Time
	method    string // Locked var: change only from method
}

func NewPayment(amount int64, currency, method string) (*Payment, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	return &Payment{
		Amount:    amount,
		Currency:  currency,
		Status:    StatusNew,
		CreatedAt: time.Now(),
		method:    method,
	}, nil
}

func (p *Payment) Method() string { return p.method }
