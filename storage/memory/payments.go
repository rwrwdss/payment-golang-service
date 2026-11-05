// Package memory хранит данные в памяти процесса.
// Подходит для тестов и локального запуска; после перезапуска данные пропадают.
package memory

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"payment-service/payments"
)

var _ payments.Repository = (*PaymentRepository)(nil)

// PaymentRepository хранит платежи в map и безопасен для параллельного доступа.
// Внутри лежат копии: изменения платежа попадают в хранилище только через Save.
type PaymentRepository struct {
	mu       sync.RWMutex
	items    map[uuid.UUID]payments.Payment
	requests map[string]requestRecord
}

type requestRecord struct {
	paymentID   uuid.UUID
	requestHash string
}

// NewPaymentRepository создаёт пустое хранилище.
func NewPaymentRepository() *PaymentRepository {
	return &PaymentRepository{
		items:    make(map[uuid.UUID]payments.Payment),
		requests: make(map[string]requestRecord),
	}
}

// Save сохраняет платёж. Повторный вызов перезаписывает прежнюю версию.
func (r *PaymentRepository) Save(_ context.Context, p *payments.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[p.ID()] = *p
	return nil
}

// GetByID возвращает копию сохранённого платежа или payments.ErrNotFound.
func (r *PaymentRepository) GetByID(_ context.Context, id uuid.UUID) (*payments.Payment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.items[id]
	if !ok {
		return nil, payments.ErrNotFound
	}
	return &p, nil
}

// CreateWithIdempotency stores the first request or returns the payment it created earlier.
func (r *PaymentRepository) CreateWithIdempotency(ctx context.Context, p *payments.Payment, key, requestHash string) (*payments.Payment, error) {
	if key == "" {
		if err := r.Save(ctx, p); err != nil {
			return nil, err
		}
		return p, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if previous, ok := r.requests[key]; ok {
		if previous.requestHash != requestHash {
			return nil, payments.ErrIdempotencyConflict
		}
		stored, ok := r.items[previous.paymentID]
		if !ok {
			return nil, payments.ErrNotFound
		}
		return &stored, nil
	}

	r.items[p.ID()] = *p
	r.requests[key] = requestRecord{paymentID: p.ID(), requestHash: requestHash}
	return p, nil
}
