package memory

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"payment-service/payments"
)

func TestPaymentRepositoryGetSaved(t *testing.T) {
	repo := NewPaymentRepository()
	saved := newPayment(t)

	if err := repo.Save(context.Background(), saved); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(context.Background(), saved.ID())

	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID() != saved.ID() || got.Amount() != saved.Amount() || got.Status() != saved.Status() {
		t.Errorf("got payment %s/%d/%s, want %s/%d/%s",
			got.ID(), got.Amount(), got.Status(), saved.ID(), saved.Amount(), saved.Status())
	}
}

func TestPaymentRepositoryGetUnknown(t *testing.T) {
	repo := NewPaymentRepository()

	_, err := repo.GetByID(context.Background(), uuid.New())

	if !errors.Is(err, payments.ErrNotFound) {
		t.Errorf("err = %v, want %v", err, payments.ErrNotFound)
	}
}

func TestPaymentRepositorySaveOverwrites(t *testing.T) {
	repo := NewPaymentRepository()
	p := newPayment(t)
	mustSave(t, repo, p)

	if err := p.StartProcessing(); err != nil {
		t.Fatalf("StartProcessing: %v", err)
	}
	mustSave(t, repo, p)
	mustSave(t, repo, p)
	got, err := repo.GetByID(context.Background(), p.ID())

	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status() != payments.StatusWaiting {
		t.Errorf("Status() = %q, want %q", got.Status(), payments.StatusWaiting)
	}
}

func TestPaymentRepositoryKeepsChangesUntilSave(t *testing.T) {
	repo := NewPaymentRepository()
	p := newPayment(t)
	mustSave(t, repo, p)

	if err := p.StartProcessing(); err != nil {
		t.Fatalf("StartProcessing: %v", err)
	}
	loaded, err := repo.GetByID(context.Background(), p.ID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if err := loaded.StartProcessing(); err != nil {
		t.Fatalf("StartProcessing on loaded: %v", err)
	}
	got, err := repo.GetByID(context.Background(), p.ID())

	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status() != payments.StatusNew {
		t.Errorf("Status() = %q, want %q: unsaved changes leaked into storage", got.Status(), payments.StatusNew)
	}
}

func TestPaymentRepositoryConcurrentAccess(t *testing.T) {
	const workers = 50
	repo := NewPaymentRepository()
	var wg sync.WaitGroup

	for range workers {
		p := newPayment(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := repo.Save(context.Background(), p); err != nil {
				t.Errorf("Save: %v", err)
				return
			}
			if _, err := repo.GetByID(context.Background(), p.ID()); err != nil {
				t.Errorf("GetByID: %v", err)
			}
		}()
	}
	wg.Wait()
}

func newPayment(t *testing.T) *payments.Payment {
	t.Helper()

	p, err := payments.NewPayment(15000, payments.CurrencyRUB, payments.MethodCard)
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	return p
}

func mustSave(t *testing.T, repo *PaymentRepository, p *payments.Payment) {
	t.Helper()

	if err := repo.Save(context.Background(), p); err != nil {
		t.Fatalf("Save: %v", err)
	}
}
