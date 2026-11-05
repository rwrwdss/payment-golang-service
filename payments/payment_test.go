package payments

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewPayment(t *testing.T) {
	tests := []struct {
		name        string
		amount      int64
		currency    Currency
		method      Method
		description string
		wantErr     error
	}{
		{"valid", 15000, CurrencyRUB, MethodCard, "Заказ №1", nil},
		{"empty description", 15000, CurrencyRUB, MethodSBP, "", nil},
		{"description at limit", 15000, CurrencyRUB, MethodCard, strings.Repeat("я", 128), nil},
		{"zero amount", 0, CurrencyRUB, MethodCard, "", ErrInvalidAmount},
		{"negative amount", -1, CurrencyRUB, MethodCard, "", ErrInvalidAmount},
		{"unknown currency", 15000, "BTC", MethodCard, "", ErrInvalidCurrency},
		{"unknown method", 15000, CurrencyRUB, "cash", "", ErrInvalidMethod},
		{"description too long", 15000, CurrencyRUB, MethodCard, strings.Repeat("я", 129), ErrInvalidDescription},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewPayment(tt.amount, tt.currency, tt.method, tt.description)

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
			if p.Description() != tt.description {
				t.Errorf("Description() = %q, want %q", p.Description(), tt.description)
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

func TestPaymentRecordRoundTrip(t *testing.T) {
	p, err := NewPayment(15000, CurrencyRUB, MethodCard, "Заказ №1")
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	if err := p.Submit("provider-id", "https://yoomoney.ru/checkout"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := p.Cancel("insufficient_funds"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	restored, err := RestorePayment(p.Record())
	if err != nil {
		t.Fatalf("RestorePayment: %v", err)
	}
	if restored.Record() != p.Record() {
		t.Errorf("restored record = %+v, want %+v", restored.Record(), p.Record())
	}
}

func TestRestoreCanceledPaymentWithoutProviderID(t *testing.T) {
	p, err := NewPayment(100, CurrencyRUB, MethodCard, "Order 2")
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	if err := p.Cancel("provider rejected request"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if _, err := RestorePayment(p.Record()); err != nil {
		t.Fatalf("RestorePayment: %v", err)
	}
}

func TestRestorePaymentRejectsInvalidRecord(t *testing.T) {
	_, err := RestorePayment(PaymentRecord{Status: "unknown"})

	if !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidRecord)
	}
}

func TestPaymentSubmit(t *testing.T) {
	p := newPaymentIn(t, StatusNew)

	err := p.Submit("2d7a1c3e-000f-5000-9000-1b2c3d4e5f60", "https://yoomoney.ru/checkout/payments/v2/contract?orderId=2d7a")

	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if p.Status() != StatusPending {
		t.Errorf("Status() = %q, want %q", p.Status(), StatusPending)
	}
	if p.ProviderID() != "2d7a1c3e-000f-5000-9000-1b2c3d4e5f60" {
		t.Errorf("ProviderID() = %q", p.ProviderID())
	}
	if p.ConfirmationURL() != "https://yoomoney.ru/checkout/payments/v2/contract?orderId=2d7a" {
		t.Errorf("ConfirmationURL() = %q", p.ConfirmationURL())
	}
}

func TestPaymentSubmitRejectsEmptyProviderID(t *testing.T) {
	p := newPaymentIn(t, StatusNew)

	err := p.Submit("", "https://yoomoney.ru/checkout")

	if !errors.Is(err, ErrMissingProviderID) {
		t.Fatalf("err = %v, want %v", err, ErrMissingProviderID)
	}
	if p.Status() != StatusNew {
		t.Errorf("Status() = %q, want %q", p.Status(), StatusNew)
	}
}

func TestPaymentSubmitTwiceKeepsFirstProviderData(t *testing.T) {
	p := newPaymentIn(t, StatusPending)
	providerID := p.ProviderID()

	err := p.Submit("another-id", "https://yoomoney.ru/other")

	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidTransition)
	}
	if p.ProviderID() != providerID {
		t.Errorf("ProviderID() = %q, want %q", p.ProviderID(), providerID)
	}
}

func TestPaymentCancelKeepsReason(t *testing.T) {
	p := newPaymentIn(t, StatusPending)

	if err := p.Cancel("insufficient_funds"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if p.Status() != StatusCanceled {
		t.Errorf("Status() = %q, want %q", p.Status(), StatusCanceled)
	}
	if p.CancelReason() != "insufficient_funds" {
		t.Errorf("CancelReason() = %q, want %q", p.CancelReason(), "insufficient_funds")
	}
}

func TestPaymentActions(t *testing.T) {
	submit := func(p *Payment) error { return p.Submit("provider-id", "https://yoomoney.ru/checkout") }
	cancel := func(p *Payment) error { return p.Cancel("canceled_by_merchant") }

	tests := []struct {
		name    string
		from    Status
		action  func(*Payment) error
		want    Status
		wantErr error
	}{
		{"submit new", StatusNew, submit, StatusPending, nil},
		{"cancel new", StatusNew, cancel, StatusCanceled, nil},
		{"succeed pending", StatusPending, (*Payment).Succeed, StatusSucceeded, nil},
		{"cancel pending", StatusPending, cancel, StatusCanceled, nil},
		{"succeed new", StatusNew, (*Payment).Succeed, StatusNew, ErrInvalidTransition},
		{"submit pending", StatusPending, submit, StatusPending, ErrInvalidTransition},
		{"cancel succeeded", StatusSucceeded, cancel, StatusSucceeded, ErrInvalidTransition},
		{"succeed canceled", StatusCanceled, (*Payment).Succeed, StatusCanceled, ErrInvalidTransition},
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

	submit := func(p *Payment) error { return p.Submit(uuid.NewString(), "https://yoomoney.ru/checkout") }
	steps := map[Status][]func(*Payment) error{
		StatusNew:       nil,
		StatusPending:   {submit},
		StatusSucceeded: {submit, (*Payment).Succeed},
		StatusCanceled:  {submit, func(p *Payment) error { return p.Cancel("general_decline") }},
	}

	p, err := NewPayment(15000, CurrencyRUB, MethodCard, "Заказ №1")
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
