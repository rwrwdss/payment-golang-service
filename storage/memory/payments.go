// Package memory хранит данные в памяти процесса.
// Подходит для тестов и локального запуска; после перезапуска данные пропадают.
package memory

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"payment-service/payments"
)

// PaymentRepository хранит платежи в map и безопасен для параллельного доступа.
// Внутри лежат копии: изменения платежа попадают в хранилище только через Save.
type PaymentRepository struct {
	mu    sync.RWMutex
	items map[uuid.UUID]payments.Payment
}

// NewPaymentRepository создаёт пустое хранилище.
func NewPaymentRepository() *PaymentRepository {
	return &PaymentRepository{items: make(map[uuid.UUID]payments.Payment)}
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
