package yookassa_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"payment-service/payments"
	"payment-service/yookassa"
)

const returnURL = "https://shop.example/orders/complete"

func TestGatewayCreatePayment(t *testing.T) {
	tests := []struct {
		name       string
		amount     int64
		method     payments.Method
		wantValue  string
		wantMethod string
	}{
		{"card", 15000, payments.MethodCard, "150.00", "bank_card"},
		{"sbp", 9905, payments.MethodSBP, "99.05", "sbp"},
		{"one kopeck", 1, payments.MethodCard, "0.01", "bank_card"},
		{"large amount", 123456789, payments.MethodCard, "1234567.89", "bank_card"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotKey string
			var gotBody yookassa.CreatePaymentRequest
			gateway := newGateway(t, func(w http.ResponseWriter, r *http.Request) {
				gotKey = r.Header.Get("Idempotence-Key")
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Errorf("decode request body: %v", err)
				}
				respond(w, http.StatusOK, `{
					"id": "2d7a1c3e-000f-5000-9000-1b2c3d4e5f60",
					"status": "pending",
					"confirmation": {"type": "redirect", "confirmation_url": "https://yoomoney.ru/checkout/payments/v2/contract?orderId=2d7a"}
				}`)
			})
			p := mustNewPayment(t, tt.amount, tt.method)

			checkout, err := gateway.CreatePayment(context.Background(), p)

			if err != nil {
				t.Fatalf("CreatePayment: %v", err)
			}
			if checkout.ProviderID != "2d7a1c3e-000f-5000-9000-1b2c3d4e5f60" {
				t.Errorf("ProviderID = %q", checkout.ProviderID)
			}
			if checkout.ConfirmationURL != "https://yoomoney.ru/checkout/payments/v2/contract?orderId=2d7a" {
				t.Errorf("ConfirmationURL = %q", checkout.ConfirmationURL)
			}
			if gotKey != p.ID().String() {
				t.Errorf("Idempotence-Key = %q, want payment id %s", gotKey, p.ID())
			}
			assertRequest(t, gotBody, p, tt.wantValue, tt.wantMethod)
		})
	}
}

func TestGatewayCreatePaymentErrors(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		wantRejected bool
	}{
		{"invalid request is final", http.StatusBadRequest, true},
		{"bad credentials is our config problem, not a decline", http.StatusUnauthorized, false},
		{"forbidden is our config problem, not a decline", http.StatusForbidden, false},
		{"rate limit can be retried", http.StatusTooManyRequests, false},
		{"internal error can be retried", http.StatusInternalServerError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newGateway(t, func(w http.ResponseWriter, _ *http.Request) {
				respond(w, tt.status, `{"type":"error","code":"some_code"}`)
			})

			_, err := gateway.CreatePayment(context.Background(), mustNewPayment(t, 15000, payments.MethodCard))

			if err == nil {
				t.Fatal("CreatePayment returned no error")
			}
			if got := errors.Is(err, payments.ErrPaymentRejected); got != tt.wantRejected {
				t.Errorf("errors.Is(err, ErrPaymentRejected) = %v, want %v (err: %v)", got, tt.wantRejected, err)
			}
			var apiErr *yookassa.APIError
			if !errors.As(err, &apiErr) {
				t.Errorf("original APIError is lost: %v", err)
			}
		})
	}
}

func TestGatewayFetchPayment(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus payments.Status
		wantReason string
	}{
		{"pending", `{"id":"yk-1","status":"pending"}`, payments.StatusPending, ""},
		{"waiting for capture", `{"id":"yk-1","status":"waiting_for_capture"}`, payments.StatusPending, ""},
		{"succeeded", `{"id":"yk-1","status":"succeeded","paid":true}`, payments.StatusSucceeded, ""},
		{
			"canceled with reason",
			`{"id":"yk-1","status":"canceled","cancellation_details":{"party":"payment_network","reason":"insufficient_funds"}}`,
			payments.StatusCanceled,
			"insufficient_funds",
		},
		{"canceled without details", `{"id":"yk-1","status":"canceled"}`, payments.StatusCanceled, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			gateway := newGateway(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				respond(w, http.StatusOK, tt.body)
			})

			state, err := gateway.FetchPayment(context.Background(), "yk-1")

			if err != nil {
				t.Fatalf("FetchPayment: %v", err)
			}
			if gotPath != "/payments/yk-1" {
				t.Errorf("path = %q, want /payments/yk-1", gotPath)
			}
			if state.Status != tt.wantStatus || state.CancelReason != tt.wantReason {
				t.Errorf("state = %+v, want %s/%q", state, tt.wantStatus, tt.wantReason)
			}
		})
	}
}

func TestGatewayFetchPaymentErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"unknown status", http.StatusOK, `{"id":"yk-1","status":"refunding"}`},
		{"api error", http.StatusNotFound, `{"type":"error","code":"not_found"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newGateway(t, func(w http.ResponseWriter, _ *http.Request) {
				respond(w, tt.status, tt.body)
			})

			if _, err := gateway.FetchPayment(context.Background(), "yk-1"); err == nil {
				t.Error("FetchPayment returned no error")
			}
		})
	}
}

func assertRequest(t *testing.T, got yookassa.CreatePaymentRequest, p *payments.Payment, wantValue, wantMethod string) {
	t.Helper()

	if got.Amount.Value != wantValue || got.Amount.Currency != string(p.Currency()) {
		t.Errorf("amount = %+v, want %s %s", got.Amount, wantValue, p.Currency())
	}
	if !got.Capture {
		t.Error("capture = false, want one-stage payment")
	}
	if got.Confirmation.Type != yookassa.ConfirmationRedirect || got.Confirmation.ReturnURL != returnURL {
		t.Errorf("confirmation = %+v", got.Confirmation)
	}
	if got.PaymentMethodData == nil || got.PaymentMethodData.Type != wantMethod {
		t.Errorf("payment_method_data = %+v, want %s", got.PaymentMethodData, wantMethod)
	}
	if got.Description != p.Description() {
		t.Errorf("description = %q, want %q", got.Description, p.Description())
	}
	if got.Metadata["payment_id"] != p.ID().String() {
		t.Errorf("metadata = %v, want payment_id %s", got.Metadata, p.ID())
	}
}

func newGateway(t *testing.T, handler http.HandlerFunc) *yookassa.Gateway {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := yookassa.NewClient(yookassa.Config{ShopID: "100500", SecretKey: "test_secret", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return yookassa.NewGateway(client, returnURL)
}

func mustNewPayment(t *testing.T, amount int64, method payments.Method) *payments.Payment {
	t.Helper()

	p, err := payments.NewPayment(amount, payments.CurrencyRUB, method, fmt.Sprintf("Заказ на %d коп.", amount))
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	return p
}

func respond(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
