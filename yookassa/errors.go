package yookassa

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// maxErrorBody — сколько байт тела ошибки читаем. Больше нам не нужно,
// а прокси перед ЮKassa иногда отдают огромные HTML-страницы.
const maxErrorBody = 64 << 10

// APIError — ответ ЮKassa с кодом, отличным от 200.
type APIError struct {
	StatusCode  int    `json:"-"`
	ID          string `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Parameter   string `json:"parameter"`
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("yookassa: %d %s", e.StatusCode, e.Code)
	if e.Description != "" {
		msg += ": " + e.Description
	}
	if e.Parameter != "" {
		msg += " (" + e.Parameter + ")"
	}
	return msg
}

// Temporary сообщает, что запрос можно повторить с тем же ключом идемпотентности.
// По документации ЮKassa это 429 и 5xx: при 500 операция могла как выполниться, так и нет.
func (e *APIError) Temporary() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= http.StatusInternalServerError
}

func decodeAPIError(resp *http.Response) error {
	apiErr := &APIError{StatusCode: resp.StatusCode}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil || json.Unmarshal(body, apiErr) != nil {
		apiErr.Description = http.StatusText(resp.StatusCode)
	}
	return apiErr
}
