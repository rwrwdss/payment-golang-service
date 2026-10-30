package payments

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewPayment(t *testing.T) {
	tests := []struct {
		name     string
		amount   int64
		currency Currency
		method   Method
		wantErr  error
	}{
		{"valid", 15000, CurrencyRUB, MethodCard, nil},
		{"zero amount", 0, CurrencyRUB, MethodCard, ErrInvalidAmount},
		{"negative amount", -1, CurrencyRUB, MethodCard, ErrInvalidAmount},
		{"unknown currency", 15000, "BTC", MethodCard, ErrInvalidCurrency},
		{"unknown method", 15000, CurrencyRUB, "cash", ErrInvalidMethod},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewPayment(tt.amount, tt.currency, tt.method)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			if p.ID() == uuid.Nil {
				t.Error("ID is empty")
			}
			if p.Amount() != tt.amount {
				t.Errorf("Amount() = %d, want %d", p.Amount(), tt.amount)
			}
			if p.Currency() != tt.currency {
				t.Errorf("Currency() = %q, want %q", p.Currency(), tt.currency)
			}
			if p.Method() != tt.method {
				t.Errorf("Method() = %q, want %q", p.Method(), tt.method)
			}
			if p.Status() != StatusNew {
				t.Errorf("Status() = %q, want %q", p.Status(), StatusNew)
			}
			if !p.CreatedAt().Equal(p.UpdatedAt()) {
				t.Errorf("CreatedAt() = %v, UpdatedAt() = %v, want equal", p.CreatedAt(), p.UpdatedAt())
			}
		})
	}
}

func TestNewPaymentGeneratesUniqueIDs(t *testing.T) {
	first := newPaymentIn(t, StatusNew)
	second := newPaymentIn(t, StatusNew)

	if first.ID() == second.ID() {
		t.Errorf("two payments got the same ID %s", first.ID())
	}
}

func TestPaymentActions(t *testing.T) {
	tests := []struct {
		name    string
		from    Status
		action  func(*Payment) error
		want    Status
		wantErr error
	}{
		{"start processing new", StatusNew, (*Payment).StartProcessing, StatusWaiting, nil},
		{"succeed waiting", StatusWaiting, (*Payment).Succeed, StatusSuccess, nil},
		{"deny waiting", StatusWaiting, (*Payment).Deny, StatusDenied, nil},
		{"succeed new", StatusNew, (*Payment).Succeed, StatusNew, ErrInvalidTransition},
		{"deny new", StatusNew, (*Payment).Deny, StatusNew, ErrInvalidTransition},
		{"start processing twice", StatusWaiting, (*Payment).StartProcessing, StatusWaiting, ErrInvalidTransition},
		{"deny success", StatusSuccess, (*Payment).Deny, StatusSuccess, ErrInvalidTransition},
		{"succeed denied", StatusDenied, (*Payment).Succeed, StatusDenied, ErrInvalidTransition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPaymentIn(t, tt.from)
			updatedBefore := p.UpdatedAt()
			actionStart := time.Now()

			err := tt.action(p)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if p.Status() != tt.want {
				t.Errorf("Status() = %q, want %q", p.Status(), tt.want)
			}
			if err != nil && !p.UpdatedAt().Equal(updatedBefore) {
				t.Error("UpdatedAt() changed after a rejected transition")
			}
			if err == nil && p.UpdatedAt().Before(actionStart) {
				t.Error("UpdatedAt() was not refreshed by the transition")
			}
		})
	}
}

// newPaymentIn создаёт платёж и доводит его до нужного статуса
// через публичные методы, как это сделал бы вызывающий код.
func newPaymentIn(t *testing.T, status Status) *Payment {
	t.Helper()

	steps := map[Status][]func(*Payment) error{
		StatusNew:     nil,
		StatusWaiting: {(*Payment).StartProcessing},
		StatusSuccess: {(*Payment).StartProcessing, (*Payment).Succeed},
		StatusDenied:  {(*Payment).StartProcessing, (*Payment).Deny},
	}

	p, err := NewPayment(15000, CurrencyRUB, MethodCard)
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	for _, step := range steps[status] {
		if err := step(p); err != nil {
			t.Fatalf("move payment to %s: %v", status, err)
		}
	}
	return p
}
