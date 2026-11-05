package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"payment-service/payments"
	"payment-service/storage/memory"
)

func TestCreatePaymentRequiresIdempotencyKey(t *testing.T) {
	gateway := &webhookGateway{}
	svc := payments.NewService(memory.NewPaymentRepository(), gateway)
	request := httptest.NewRequest(http.MethodPost, "/payments", strings.NewReader(`{"amount":15000,"currency":"RUB","method":"card"}`))
	response := httptest.NewRecorder()

	Handler(svc).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if gateway.createCalls != 0 {
		t.Errorf("provider create called %d times, want 0", gateway.createCalls)
	}
}

func TestCreatePaymentReusesIdempotencyKey(t *testing.T) {
	gateway := &webhookGateway{}
	svc := payments.NewService(memory.NewPaymentRepository(), gateway)
	handler := Handler(svc)
	body := `{"amount":15000,"currency":"RUB","method":"card","description":"Order 1"}`

	first := sendCreate(handler, body, "shop-order-1")
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d; body: %s", first.Code, http.StatusCreated, first.Body)
	}
	var firstPayment paymentResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstPayment); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	duplicate := sendCreate(handler, body, "shop-order-1")
	if duplicate.Code != http.StatusCreated {
		t.Fatalf("duplicate status = %d, want %d; body: %s", duplicate.Code, http.StatusCreated, duplicate.Body)
	}
	var duplicatePayment paymentResponse
	if err := json.Unmarshal(duplicate.Body.Bytes(), &duplicatePayment); err != nil {
		t.Fatalf("decode duplicate response: %v", err)
	}
	if duplicatePayment.ID != firstPayment.ID {
		t.Fatalf("duplicate id = %s, want %s", duplicatePayment.ID, firstPayment.ID)
	}
	if gateway.createCalls != 1 {
		t.Fatalf("provider create called %d times, want 1", gateway.createCalls)
	}

	conflict := sendCreate(handler, `{"amount":15001,"currency":"RUB","method":"card","description":"Order 1"}`, "shop-order-1")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want %d", conflict.Code, http.StatusConflict)
	}
}

func sendCreate(handler http.Handler, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/payments", strings.NewReader(body))
	req.Header.Set("Idempotency-Key", key)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
