package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"payment-service/payments"
)

func TestPaymentRepositorySaveAndGet(t *testing.T) {
	db := testDatabase(t)
	repo := NewPaymentRepository(db)

	want, err := payments.NewPayment(1599, payments.CurrencyRUB, payments.MethodSBP, "Order 12")
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	if err := repo.Save(context.Background(), want); err != nil {
		t.Fatalf("Save new payment: %v", err)
	}

	got, err := repo.GetByID(context.Background(), want.ID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	gotRecord, wantRecord := got.Record(), want.Record()
	if !withinMicrosecond(gotRecord.CreatedAt, wantRecord.CreatedAt) || !withinMicrosecond(gotRecord.UpdatedAt, wantRecord.UpdatedAt) {
		t.Fatalf("timestamps changed: got %s/%s, want %s/%s", gotRecord.CreatedAt, gotRecord.UpdatedAt, wantRecord.CreatedAt, wantRecord.UpdatedAt)
	}
	gotRecord.CreatedAt, gotRecord.UpdatedAt = wantRecord.CreatedAt, wantRecord.UpdatedAt
	if gotRecord != wantRecord {
		t.Fatalf("record = %+v, want %+v", got.Record(), want.Record())
	}

	if err := got.Submit("provider-"+uuid.NewString(), "https://pay.example/12"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := repo.Save(context.Background(), got); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	updated, err := repo.GetByID(context.Background(), want.ID())
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if updated.Status() != payments.StatusPending || updated.ConfirmationURL() != "https://pay.example/12" {
		t.Fatalf("updated payment = %+v", updated.Record())
	}
}

func TestPaymentRepositoryNotFound(t *testing.T) {
	repo := NewPaymentRepository(testDatabase(t))

	_, err := repo.GetByID(context.Background(), uuid.New())

	if err != payments.ErrNotFound {
		t.Fatalf("err = %v, want %v", err, payments.ErrNotFound)
	}
}

func TestPaymentRepositoryIdempotency(t *testing.T) {
	db := testDatabase(t)
	repo := NewPaymentRepository(db)
	ctx := context.Background()
	key := uuid.NewString()
	first := newTestPayment(t)
	second := newTestPayment(t)

	created, err := repo.CreateWithIdempotency(ctx, first, key, "request-hash")
	if err != nil || created.ID() != first.ID() {
		t.Fatalf("first call = %v/%v", created, err)
	}

	reused, err := repo.CreateWithIdempotency(ctx, second, key, "request-hash")
	if err != nil || reused.ID() != first.ID() {
		t.Fatalf("duplicate call = %v/%v, want original payment", reused, err)
	}

	_, err = repo.CreateWithIdempotency(ctx, second, key, "different-hash")
	if err != payments.ErrIdempotencyConflict {
		t.Fatalf("err = %v, want %v", err, payments.ErrIdempotencyConflict)
	}
}

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return db
}

func newTestPayment(t *testing.T) *payments.Payment {
	t.Helper()

	p, err := payments.NewPayment(1599, payments.CurrencyRUB, payments.MethodSBP, "Order 12")
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	return p
}

func withinMicrosecond(first, second time.Time) bool {
	difference := first.Sub(second)
	if difference < 0 {
		difference = -difference
	}
	return difference < time.Microsecond
}
