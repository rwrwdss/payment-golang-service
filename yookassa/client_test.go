package yookassa

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	testShopID    = "100500"
	testSecretKey = "test_secret"
)

func TestNewClientRequiresCredentials(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"no shop id", Config{SecretKey: testSecretKey}},
		{"no secret key", Config{ShopID: testShopID}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewClient(tt.cfg); err == nil {
				t.Error("NewClient accepted incomplete credentials")
			}
		})
	}
}

func TestNewClientDefaults(t *testing.T) {
	client, err := NewClient(Config{ShopID: testShopID, SecretKey: testSecretKey})

	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.baseURL != DefaultBaseURL || client.http.Timeout != defaultTimeout {
		t.Errorf("baseURL = %q, timeout = %v", client.baseURL, client.http.Timeout)
	}
}

func TestClientInvalidBaseURL(t *testing.T) {
	client, err := NewClient(Config{ShopID: testShopID, SecretKey: testSecretKey, BaseURL: "://no-scheme"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.GetPayment(context.Background(), "2d7a1c3e")

	if err == nil {
		t.Error("request with a broken base URL succeeded")
	}
}

func TestClientCreatePaymentAPIError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest, `{"type":"error","code":"invalid_request","parameter":"amount.value"}`)
	})

	p, err := client.CreatePayment(context.Background(), "8f2c0e4a", CreatePaymentRequest{})

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Parameter != "amount.value" {
		t.Fatalf("err = %v, want APIError for amount.value", err)
	}
	if p != nil {
		t.Errorf("payment = %+v, want nil", p)
	}
}

func TestClientCreatePayment(t *testing.T) {
	var got struct {
		method, path, idempotenceKey, contentType string
		user, pass                                string
		authOK                                    bool
		body                                      CreatePaymentRequest
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.idempotenceKey = r.Header.Get("Idempotence-Key")
		got.contentType = r.Header.Get("Content-Type")
		got.user, got.pass, got.authOK = r.BasicAuth()
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		writeJSON(w, http.StatusOK, `{
			"id": "2d7a1c3e-000f-5000-9000-1b2c3d4e5f60",
			"status": "pending",
			"paid": false,
			"amount": {"value": "150.00", "currency": "RUB"},
			"confirmation": {"type": "redirect", "confirmation_url": "https://yoomoney.ru/checkout/payments/v2/contract?orderId=2d7a"},
			"created_at": "2026-10-30T09:15:42.524Z",
			"metadata": {"payment_id": "8f2c0e4a-6b1d-4c3e-9a7f-2b5d8e1c4a90"},
			"test": true
		}`)
	})
	req := CreatePaymentRequest{
		Amount:       Amount{Value: "150.00", Currency: "RUB"},
		Capture:      true,
		Confirmation: Confirmation{Type: ConfirmationRedirect, ReturnURL: "https://shop.example/orders/1"},
		Description:  "Заказ №1",
	}

	p, err := client.CreatePayment(context.Background(), "8f2c0e4a-6b1d-4c3e-9a7f-2b5d8e1c4a90", req)

	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/payments" {
		t.Errorf("request = %s %s, want POST /payments", got.method, got.path)
	}
	if !got.authOK || got.user != testShopID || got.pass != testSecretKey {
		t.Errorf("basic auth = %q:%q (ok=%v)", got.user, got.pass, got.authOK)
	}
	if got.idempotenceKey != "8f2c0e4a-6b1d-4c3e-9a7f-2b5d8e1c4a90" {
		t.Errorf("Idempotence-Key = %q", got.idempotenceKey)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q", got.contentType)
	}
	if got.body.Amount != req.Amount || got.body.Description != req.Description || !got.body.Capture {
		t.Errorf("request body = %+v, want %+v", got.body, req)
	}
	if p.ID != "2d7a1c3e-000f-5000-9000-1b2c3d4e5f60" || p.Status != StatusPending {
		t.Errorf("payment = %s/%s", p.ID, p.Status)
	}
	if p.Confirmation == nil || p.Confirmation.ConfirmationURL == "" {
		t.Error("confirmation_url is missing")
	}
	if !p.Test || p.Metadata["payment_id"] != "8f2c0e4a-6b1d-4c3e-9a7f-2b5d8e1c4a90" {
		t.Errorf("test = %v, metadata = %v", p.Test, p.Metadata)
	}
}

func TestClientGetPayment(t *testing.T) {
	var gotMethod, gotPath, gotKey string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotKey = r.Method, r.URL.Path, r.Header.Get("Idempotence-Key")
		writeJSON(w, http.StatusOK, `{
			"id": "2d7a1c3e-000f-5000-9000-1b2c3d4e5f60",
			"status": "canceled",
			"paid": false,
			"amount": {"value": "150.00", "currency": "RUB"},
			"cancellation_details": {"party": "payment_network", "reason": "insufficient_funds"}
		}`)
	})

	p, err := client.GetPayment(context.Background(), "2d7a1c3e-000f-5000-9000-1b2c3d4e5f60")

	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/payments/2d7a1c3e-000f-5000-9000-1b2c3d4e5f60" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotKey != "" {
		t.Errorf("GET must not send Idempotence-Key, got %q", gotKey)
	}
	if p.Status != StatusCanceled || p.CancellationDetails == nil || p.CancellationDetails.Reason != "insufficient_funds" {
		t.Errorf("payment = %+v", p)
	}
}

func TestClientAPIErrors(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		body          string
		wantCode      string
		wantTemporary bool
	}{
		{
			"invalid request",
			http.StatusBadRequest,
			`{"type":"error","id":"ab1c","code":"invalid_request","description":"Value of amount is invalid","parameter":"amount.value"}`,
			"invalid_request",
			false,
		},
		{
			"bad credentials",
			http.StatusUnauthorized,
			`{"type":"error","id":"ab1d","code":"invalid_credentials","description":"Login or password is invalid"}`,
			"invalid_credentials",
			false,
		},
		{
			"rate limit",
			http.StatusTooManyRequests,
			`{"type":"error","id":"ab1e","code":"too_many_requests","description":"Too many requests"}`,
			"too_many_requests",
			true,
		},
		{"internal error", http.StatusInternalServerError, `{"type":"error","code":"internal_server_error"}`, "internal_server_error", true},
		{"gateway timeout without json", http.StatusGatewayTimeout, `<html>504 Gateway Time-out</html>`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tt.status, tt.body)
			})

			_, err := client.GetPayment(context.Background(), "2d7a1c3e")

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want *APIError", err)
			}
			if apiErr.StatusCode != tt.status || apiErr.Code != tt.wantCode {
				t.Errorf("APIError = %d/%q, want %d/%q", apiErr.StatusCode, apiErr.Code, tt.status, tt.wantCode)
			}
			if apiErr.Temporary() != tt.wantTemporary {
				t.Errorf("Temporary() = %v, want %v", apiErr.Temporary(), tt.wantTemporary)
			}
			if apiErr.Error() == "" {
				t.Error("Error() is empty")
			}
		})
	}
}

func TestClientRespectsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
			writeJSON(w, http.StatusOK, `{"id":"2d7a1c3e","status":"pending"}`)
		}
	}))
	defer srv.Close()
	client, err := NewClient(Config{ShopID: testShopID, SecretKey: testSecretKey, BaseURL: srv.URL, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.GetPayment(context.Background(), "2d7a1c3e")

	var apiErr *APIError
	if err == nil || errors.As(err, &apiErr) {
		t.Errorf("err = %v, want a transport timeout", err)
	}
}

func TestClientBrokenResponseBody(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"id": "2d7a`)
	})

	_, err := client.GetPayment(context.Background(), "2d7a1c3e")

	if err == nil {
		t.Error("broken JSON was accepted")
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := NewClient(Config{ShopID: testShopID, SecretKey: testSecretKey, BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
