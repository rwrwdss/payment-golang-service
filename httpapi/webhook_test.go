package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"payment-service/payments"
	"payment-service/storage/memory"
)

type webhookGateway struct {
	state       payments.ProviderState
	fetchErr    error
	fetchCalls  int
	createCalls int
}

func (g *webhookGateway) CreatePayment(context.Context, *payments.Payment) (payments.Checkout, error) {
	g.createCalls++
	return payments.Checkout{ProviderID: "provider-payment-1", ConfirmationURL: "https://pay.example/1"}, nil
}

func (g *webhookGateway) FetchPayment(context.Context, string) (payments.ProviderState, error) {
	g.fetchCalls++
	return g.state, g.fetchErr
}

func TestYooKassaWebhookSyncsTrustedProviderStatus(t *testing.T) {
	gateway := &webhookGateway{state: payments.ProviderState{Status: payments.StatusSucceeded}}
	svc := payments.NewService(memory.NewPaymentRepository(), gateway)
	p, err := svc.Create(context.Background(), 1200, payments.CurrencyRUB, payments.MethodCard, "Order 1")
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	handler := Handler(svc)
	body := fmt.Sprintf(`{"type":"notification","event":"payment.succeeded","object":{"id":"%s","metadata":{"payment_id":"%s"}}}`, p.ProviderID(), p.ID())

	first := sendWebhook(handler, body, "185.71.76.2:12345")
	if first.Code != http.StatusOK {
		t.Fatalf("first webhook status = %d, want %d; body: %s", first.Code, http.StatusOK, first.Body)
	}
	if gateway.fetchCalls != 1 {
		t.Fatalf("FetchPayment called %d times, want 1", gateway.fetchCalls)
	}
	got, err := svc.GetByID(context.Background(), p.ID())
	if err != nil {
		t.Fatalf("get payment: %v", err)
	}
	if got.Status() != payments.StatusSucceeded {
		t.Fatalf("payment status = %q, want %q", got.Status(), payments.StatusSucceeded)
	}

	duplicate := sendWebhook(handler, body, "185.71.76.2:12345")
	if duplicate.Code != http.StatusOK {
		t.Fatalf("duplicate webhook status = %d, want %d", duplicate.Code, http.StatusOK)
	}
	if gateway.fetchCalls != 1 {
		t.Fatalf("duplicate caused another provider fetch; calls = %d", gateway.fetchCalls)
	}
}

func TestYooKassaWebhookOnlyAcceptsKnownSourceIP(t *testing.T) {
	gateway := &webhookGateway{state: payments.ProviderState{Status: payments.StatusSucceeded}}
	svc := payments.NewService(memory.NewPaymentRepository(), gateway)
	handler := Handler(svc)

	response := sendWebhook(handler, `{"type":"notification","event":"payment.succeeded"}`, "203.0.113.10:12345")

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if gateway.fetchCalls != 0 {
		t.Errorf("provider was queried for untrusted source, calls = %d", gateway.fetchCalls)
	}
}

func TestYooKassaWebhookRetriesWhenProviderCheckFails(t *testing.T) {
	gateway := &webhookGateway{fetchErr: errors.New("provider unavailable")}
	svc := payments.NewService(memory.NewPaymentRepository(), gateway)
	p, err := svc.Create(context.Background(), 1200, payments.CurrencyRUB, payments.MethodCard, "Order 1")
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	handler := Handler(svc)
	body := fmt.Sprintf(`{"type":"notification","event":"payment.succeeded","object":{"id":"%s","metadata":{"payment_id":"%s"}}}`, p.ProviderID(), p.ID())

	response := sendWebhook(handler, body, "185.71.76.2:12345")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if p.Status() != payments.StatusPending {
		t.Errorf("payment status = %q, want %q", p.Status(), payments.StatusPending)
	}
}

func TestYooKassaWebhookIgnoresMismatchedProviderID(t *testing.T) {
	gateway := &webhookGateway{state: payments.ProviderState{Status: payments.StatusSucceeded}}
	svc := payments.NewService(memory.NewPaymentRepository(), gateway)
	p, err := svc.Create(context.Background(), 1200, payments.CurrencyRUB, payments.MethodCard, "Order 1")
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	handler := Handler(svc)
	body := fmt.Sprintf(`{"type":"notification","event":"payment.succeeded","object":{"id":"some-other-payment","metadata":{"payment_id":"%s"}}}`, p.ID())

	response := sendWebhook(handler, body, "185.71.76.2:12345")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if gateway.fetchCalls != 0 {
		t.Errorf("provider queried for mismatched id, calls = %d", gateway.fetchCalls)
	}
	if p.Status() != payments.StatusPending {
		t.Errorf("payment status = %q, want %q", p.Status(), payments.StatusPending)
	}
}

func TestYooKassaWebhookRejectsMalformedNotification(t *testing.T) {
	svc := payments.NewService(memory.NewPaymentRepository(), &webhookGateway{})
	response := sendWebhook(Handler(svc), `{"type":"event"}`, "185.71.76.2:12345")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func sendWebhook(handler http.Handler, body, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/yookassa", strings.NewReader(body))
	req.RemoteAddr = remoteAddr
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
