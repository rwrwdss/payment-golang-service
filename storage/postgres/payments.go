// Package postgres хранит платежи в PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"payment-service/payments"
)

//go:embed schema.sql
var schema string

var _ payments.Repository = (*PaymentRepository)(nil)

// PaymentRepository хранит платежи в PostgreSQL.
type PaymentRepository struct {
	db *sql.DB
}

// NewPaymentRepository создаёт репозиторий для db.
func NewPaymentRepository(db *sql.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

// EnsureSchema создаёт таблицу платежей и индексы, если их ещё нет.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create payment schema: %w", err)
	}
	return nil
}

// Save добавляет платёж или обновляет его текущее состояние.
func (r *PaymentRepository) Save(ctx context.Context, p *payments.Payment) error {
	return savePayment(ctx, r.db, p)
}

// CreateWithIdempotency сохраняет платёж и ключ запроса в одной транзакции.
func (r *PaymentRepository) CreateWithIdempotency(ctx context.Context, p *payments.Payment, key, requestHash string) (*payments.Payment, error) {
	if key == "" {
		if err := r.Save(ctx, p); err != nil {
			return nil, err
		}
		return p, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin payment transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO payment_requests (idempotency_key, request_hash, payment_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (idempotency_key) DO NOTHING`, key, requestHash, p.ID().String())
	if err != nil {
		return nil, fmt.Errorf("save idempotency key: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("check idempotency insert: %w", err)
	}
	if inserted == 0 {
		var oldHash, paymentID string
		if err := tx.QueryRowContext(ctx, `
			SELECT request_hash, payment_id FROM payment_requests WHERE idempotency_key = $1`, key,
		).Scan(&oldHash, &paymentID); err != nil {
			return nil, fmt.Errorf("read idempotency key: %w", err)
		}
		if oldHash != requestHash {
			return nil, payments.ErrIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("finish idempotency lookup: %w", err)
		}
		id, err := uuid.Parse(paymentID)
		if err != nil {
			return nil, fmt.Errorf("parse idempotent payment id: %w", err)
		}
		stored, err := r.GetByID(ctx, id)
		return stored, err
	}

	if err := savePayment(ctx, tx, p); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("finish payment transaction: %w", err)
	}
	return p, nil
}

type dbExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func savePayment(ctx context.Context, db dbExecutor, p *payments.Payment) error {
	record := p.Record()
	_, err := db.ExecContext(ctx, `
		INSERT INTO payments (
			id, amount, currency, method, description, status, provider_id,
			confirmation_url, cancel_reason, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			amount = EXCLUDED.amount,
			currency = EXCLUDED.currency,
			method = EXCLUDED.method,
			description = EXCLUDED.description,
			status = EXCLUDED.status,
			provider_id = EXCLUDED.provider_id,
			confirmation_url = EXCLUDED.confirmation_url,
			cancel_reason = EXCLUDED.cancel_reason,
			updated_at = EXCLUDED.updated_at`,
		record.ID.String(), record.Amount, string(record.Currency), string(record.Method), record.Description,
		string(record.Status), record.ProviderID, record.ConfirmationURL, record.CancelReason,
		record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save payment %s: %w", record.ID, err)
	}
	return nil
}

// GetByID возвращает сохранённый платёж или payments.ErrNotFound.
func (r *PaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*payments.Payment, error) {
	var record payments.PaymentRecord
	var paymentID, currency, method, status string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, amount, currency, method, description, status, provider_id,
		       confirmation_url, cancel_reason, created_at, updated_at
		FROM payments WHERE id = $1`, id.String()).Scan(
		&paymentID, &record.Amount, &currency, &method, &record.Description, &status,
		&record.ProviderID, &record.ConfirmationURL, &record.CancelReason,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, payments.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get payment %s: %w", id, err)
	}

	parsedID, err := uuid.Parse(paymentID)
	if err != nil {
		return nil, fmt.Errorf("parse stored payment id: %w", err)
	}
	record.ID = parsedID
	record.Currency = payments.Currency(currency)
	record.Method = payments.Method(method)
	record.Status = payments.Status(status)

	p, err := payments.RestorePayment(record)
	if err != nil {
		return nil, fmt.Errorf("restore payment %s: %w", id, err)
	}
	return p, nil
}
