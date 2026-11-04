package payments_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"payment-service/payments"
	"payment-service/storage/memory"
)

var (
	errNetwork = errors.New("connection reset by peer")
	errDisk    = errors.New("no space left on device")
)

// fakeGateway подменяет ЮKassa: возвращает заранее заданные ответы и запоминает вызовы.
// Как и настоящий провайдер, на повторный запрос с тем же ключом отдаёт тот же платёж.
type fakeGateway struct {
	createErr     error
	emptyResponse bool
	fetchErr      error
	state         payments.ProviderState
	createdFor    []uuid.UUID
	fetchCalls    int
}

func (g *fakeGateway) CreatePayment(_ context.Context, p *payments.Payment) (payments.Checkout, error) {
	g.createdFor = append(g.createdFor, p.ID())
	if g.createErr != nil {
		return payments.Checkout{}, g.createErr
	}
	if g.emptyResponse {
		return payments.Checkout{}, nil
	}
	return payments.Checkout{
		ProviderID:      "yk-" + p.ID().String(),
		ConfirmationURL: "https://yoomoney.ru/checkout/payments/v2/contract?orderId=" + p.ID().String(),
	}, nil
}

func (g *fakeGateway) FetchPayment(_ context.Context, _ string) (payments.ProviderState, error) {
	g.fetchCalls++
	return g.state, g.fetchErr
}

// flakyRepo ломает Save с указанным номером, остальные вызовы передаёт настоящему хранилищу.
type flakyRepo struct {
	*memory.PaymentRepository
	failOnSave int
	saves      int
}

func (r *flakyRepo) Save(ctx context.Context, p *payments.Payment) error {
	r.saves++
	if r.saves == r.failOnSave {
		return errDisk
	}
	return r.PaymentRepository.Save(ctx, p)
}

func TestServiceCreate(t *testing.T) {
	svc, repo, gateway := newService()

	p, err := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "Заказ №1")

	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Status() != payments.StatusPending {
		t.Errorf("Status() = %q, want %q", p.Status(), payments.StatusPending)
	}
	if p.ProviderID() != "yk-"+p.ID().String() || p.ConfirmationURL() == "" {
		t.Errorf("provider data = %q, %q", p.ProviderID(), p.ConfirmationURL())
	}
	if len(gateway.createdFor) != 1 || gateway.createdFor[0] != p.ID() {
		t.Errorf("gateway called for %v, want [%s]", gateway.createdFor, p.ID())
	}
	assertStored(t, repo, p.ID(), payments.StatusPending)
}

func TestServiceCreateInvalidInput(t *testing.T) {
	svc, _, gateway := newService()

	_, err := svc.Create(context.Background(), 0, payments.CurrencyRUB, payments.MethodCard, "")

	if !errors.Is(err, payments.ErrInvalidAmount) {
		t.Fatalf("err = %v, want %v", err, payments.ErrInvalidAmount)
	}
	if len(gateway.createdFor) != 0 {
		t.Error("invalid payment was sent to the provider")
	}
}

func TestServiceCreateRejectedByProvider(t *testing.T) {
	svc, repo, gateway := newService()
	gateway.createErr = fmt.Errorf("%w: invalid_request: amount", payments.ErrPaymentRejected)

	p, err := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")

	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Status() != payments.StatusCanceled {
		t.Errorf("Status() = %q, want %q", p.Status(), payments.StatusCanceled)
	}
	if p.CancelReason() == "" {
		t.Error("CancelReason() is empty")
	}
	assertStored(t, repo, p.ID(), payments.StatusCanceled)
}

func TestServiceCreateProviderUnavailable(t *testing.T) {
	svc, repo, gateway := newService()
	gateway.createErr = errNetwork

	p, err := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")

	if !errors.Is(err, errNetwork) {
		t.Fatalf("err = %v, want %v", err, errNetwork)
	}
	if p == nil {
		t.Fatal("payment is nil, caller cannot retry it")
	}
	assertStored(t, repo, p.ID(), payments.StatusNew)
}

func TestServiceCreateEmptyProviderResponse(t *testing.T) {
	svc, repo, gateway := newService()
	gateway.emptyResponse = true

	p, err := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")

	if !errors.Is(err, payments.ErrMissingProviderID) {
		t.Fatalf("err = %v, want %v", err, payments.ErrMissingProviderID)
	}
	if p == nil {
		t.Fatal("payment is nil, caller cannot retry it")
	}
	assertStored(t, repo, p.ID(), payments.StatusNew)
}

func TestServiceCreateStorageFailures(t *testing.T) {
	t.Run("before sending to provider", func(t *testing.T) {
		repo := &flakyRepo{PaymentRepository: memory.NewPaymentRepository(), failOnSave: 1}
		gateway := &fakeGateway{}
		svc := payments.NewService(repo, gateway)

		_, err := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")

		if !errors.Is(err, errDisk) {
			t.Fatalf("err = %v, want %v", err, errDisk)
		}
		if len(gateway.createdFor) != 0 {
			t.Error("payment was sent to the provider although it was never saved")
		}
	})

	t.Run("after provider accepted, retry finishes the job", func(t *testing.T) {
		repo := &flakyRepo{PaymentRepository: memory.NewPaymentRepository(), failOnSave: 2}
		gateway := &fakeGateway{}
		svc := payments.NewService(repo, gateway)

		p, err := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")
		if !errors.Is(err, errDisk) {
			t.Fatalf("err = %v, want %v", err, errDisk)
		}
		got, err := svc.Submit(context.Background(), p.ID())

		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if got.ProviderID() != p.ProviderID() {
			t.Errorf("retry got provider payment %q, want the same %q", got.ProviderID(), p.ProviderID())
		}
		assertStored(t, repo.PaymentRepository, p.ID(), payments.StatusPending)
	})
}

func TestServiceSubmitRetriesWithSameKey(t *testing.T) {
	svc, repo, gateway := newService()
	gateway.createErr = errNetwork
	p, _ := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")
	gateway.createErr = nil

	got, err := svc.Submit(context.Background(), p.ID())

	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.Status() != payments.StatusPending {
		t.Errorf("Status() = %q, want %q", got.Status(), payments.StatusPending)
	}
	want := []uuid.UUID{p.ID(), p.ID()}
	if len(gateway.createdFor) != 2 || gateway.createdFor[0] != want[0] || gateway.createdFor[1] != want[1] {
		t.Errorf("gateway called for %v, want %v", gateway.createdFor, want)
	}
	assertStored(t, repo, p.ID(), payments.StatusPending)
}

func TestServiceSubmitAlreadySubmitted(t *testing.T) {
	svc, _, gateway := newService()
	p, _ := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")

	got, err := svc.Submit(context.Background(), p.ID())

	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.ProviderID() != p.ProviderID() {
		t.Errorf("ProviderID() = %q, want %q", got.ProviderID(), p.ProviderID())
	}
	if len(gateway.createdFor) != 1 {
		t.Errorf("gateway called %d times, want 1", len(gateway.createdFor))
	}
}

func TestServiceSubmitUnknownPayment(t *testing.T) {
	svc, _, _ := newService()

	_, err := svc.Submit(context.Background(), uuid.New())

	if !errors.Is(err, payments.ErrNotFound) {
		t.Errorf("err = %v, want %v", err, payments.ErrNotFound)
	}
}

func TestServiceSync(t *testing.T) {
	tests := []struct {
		name       string
		state      payments.ProviderState
		wantStatus payments.Status
		wantReason string
	}{
		{"still pending", payments.ProviderState{Status: payments.StatusPending}, payments.StatusPending, ""},
		{"succeeded", payments.ProviderState{Status: payments.StatusSucceeded}, payments.StatusSucceeded, ""},
		{
			"canceled",
			payments.ProviderState{Status: payments.StatusCanceled, CancelReason: "insufficient_funds"},
			payments.StatusCanceled,
			"insufficient_funds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, gateway := newService()
			p, _ := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")
			gateway.state = tt.state

			got, err := svc.Sync(context.Background(), p.ID())

			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if got.Status() != tt.wantStatus {
				t.Errorf("Status() = %q, want %q", got.Status(), tt.wantStatus)
			}
			if got.CancelReason() != tt.wantReason {
				t.Errorf("CancelReason() = %q, want %q", got.CancelReason(), tt.wantReason)
			}
			assertStored(t, repo, p.ID(), tt.wantStatus)
		})
	}
}

func TestServiceSyncIsIdempotent(t *testing.T) {
	svc, repo, gateway := newService()
	p, _ := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")
	gateway.state = payments.ProviderState{Status: payments.StatusSucceeded}

	for range 3 {
		if _, err := svc.Sync(context.Background(), p.ID()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
	}

	if gateway.fetchCalls != 1 {
		t.Errorf("provider asked %d times, want 1: final payments must not be re-fetched", gateway.fetchCalls)
	}
	assertStored(t, repo, p.ID(), payments.StatusSucceeded)
}

func TestServiceSyncNotSubmitted(t *testing.T) {
	svc, repo, gateway := newService()
	gateway.createErr = errNetwork
	p, _ := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")

	got, err := svc.Sync(context.Background(), p.ID())

	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got.Status() != payments.StatusNew || gateway.fetchCalls != 0 {
		t.Errorf("Status() = %q, fetch calls = %d; want new and no calls", got.Status(), gateway.fetchCalls)
	}
	assertStored(t, repo, p.ID(), payments.StatusNew)
}

func TestServiceSyncProviderErrors(t *testing.T) {
	tests := []struct {
		name     string
		fetchErr error
		state    payments.ProviderState
	}{
		{"provider unavailable", errNetwork, payments.ProviderState{}},
		{"unexpected status", nil, payments.ProviderState{Status: payments.StatusNew}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, gateway := newService()
			p, _ := svc.Create(context.Background(), 15000, payments.CurrencyRUB, payments.MethodCard, "")
			gateway.fetchErr = tt.fetchErr
			gateway.state = tt.state

			_, err := svc.Sync(context.Background(), p.ID())

			if err == nil {
				t.Fatal("Sync returned no error")
			}
			if tt.fetchErr != nil && !errors.Is(err, tt.fetchErr) {
				t.Errorf("err = %v, want %v", err, tt.fetchErr)
			}
			assertStored(t, repo, p.ID(), payments.StatusPending)
		})
	}
}

func TestServiceSyncUnknownPayment(t *testing.T) {
	svc, _, _ := newService()

	_, err := svc.Sync(context.Background(), uuid.New())

	if !errors.Is(err, payments.ErrNotFound) {
		t.Errorf("err = %v, want %v", err, payments.ErrNotFound)
	}
}

func newService() (*payments.Service, *memory.PaymentRepository, *fakeGateway) {
	repo := memory.NewPaymentRepository()
	gateway := &fakeGateway{}
	return payments.NewService(repo, gateway), repo, gateway
}

func assertStored(t *testing.T, repo *memory.PaymentRepository, id uuid.UUID, want payments.Status) {
	t.Helper()

	stored, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Status() != want {
		t.Errorf("stored Status() = %q, want %q", stored.Status(), want)
	}
}
