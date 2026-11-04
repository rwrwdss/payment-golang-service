// Package httpapi предоставляет HTTP API для платежей.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"payment-service/payments"
)

const maxRequestBody = 1 << 20

// Handler возвращает HTTP-обработчики для создания и чтения платежей.
func Handler(service *payments.Service) http.Handler {
	h := &handler{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /payments", h.create)
	mux.HandleFunc("GET /payments/{id}", h.get)
	return mux
}

type handler struct {
	service *payments.Service
}

type createRequest struct {
	Amount      int64             `json:"amount"`
	Currency    payments.Currency `json:"currency"`
	Method      payments.Method   `json:"method"`
	Description string            `json:"description"`
}

type paymentResponse struct {
	ID              string            `json:"id"`
	Amount          int64             `json:"amount"`
	Currency        payments.Currency `json:"currency"`
	Method          payments.Method   `json:"method"`
	Description     string            `json:"description"`
	Status          payments.Status   `json:"status"`
	ConfirmationURL string            `json:"confirmation_url,omitempty"`
	CancelReason    string            `json:"cancel_reason,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var input createRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request must contain one JSON object")
		return
	}

	p, err := h.service.Create(r.Context(), input.Amount, input.Currency, input.Method, input.Description)
	if err != nil {
		if p != nil {
			writeJSON(w, http.StatusBadGateway, paymentResponseFrom(p))
			return
		}
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, paymentResponseFrom(p))
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid payment id")
		return
	}

	p, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, paymentResponseFrom(p))
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, payments.ErrInvalidAmount),
		errors.Is(err, payments.ErrInvalidCurrency),
		errors.Is(err, payments.ErrInvalidMethod),
		errors.Is(err, payments.ErrInvalidDescription):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, payments.ErrNotFound):
		writeError(w, http.StatusNotFound, "payment not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func paymentResponseFrom(p *payments.Payment) paymentResponse {
	return paymentResponse{
		ID:              p.ID().String(),
		Amount:          p.Amount(),
		Currency:        p.Currency(),
		Method:          p.Method(),
		Description:     p.Description(),
		Status:          p.Status(),
		ConfirmationURL: p.ConfirmationURL(),
		CancelReason:    p.CancelReason(),
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
