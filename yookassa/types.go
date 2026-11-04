package yookassa

import "time"

// Статусы платежа в ЮKassa.
const (
	StatusPending           = "pending"
	StatusWaitingForCapture = "waiting_for_capture"
	StatusSucceeded         = "succeeded"
	StatusCanceled          = "canceled"
)

// ConfirmationRedirect — сценарий, при котором клиент уходит на страницу оплаты ЮKassa
// и возвращается на return_url.
const ConfirmationRedirect = "redirect"

// Amount — сумма в формате ЮKassa: строка с точкой и двумя знаками после неё, например "150.00".
type Amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// Confirmation — как клиент подтверждает платёж.
// В запросе заполняется ReturnURL, в ответе приходит ConfirmationURL.
type Confirmation struct {
	Type            string `json:"type"`
	ReturnURL       string `json:"return_url,omitempty"`
	ConfirmationURL string `json:"confirmation_url,omitempty"`
}

// PaymentMethodData — способ оплаты, выбранный заранее. Без него клиент выберет способ на странице ЮKassa.
type PaymentMethodData struct {
	Type string `json:"type"`
}

// CancellationDetails — кто и почему отменил платёж.
type CancellationDetails struct {
	Party  string `json:"party"`
	Reason string `json:"reason"`
}

// CreatePaymentRequest — тело запроса POST /payments.
type CreatePaymentRequest struct {
	Amount            Amount             `json:"amount"`
	Capture           bool               `json:"capture"`
	Confirmation      Confirmation       `json:"confirmation"`
	PaymentMethodData *PaymentMethodData `json:"payment_method_data,omitempty"`
	Description       string             `json:"description,omitempty"`
	Metadata          map[string]string  `json:"metadata,omitempty"`
}

// Payment — объект платежа ЮKassa (только поля, которые нужны сервису).
type Payment struct {
	ID                  string               `json:"id"`
	Status              string               `json:"status"`
	Paid                bool                 `json:"paid"`
	Amount              Amount               `json:"amount"`
	Confirmation        *Confirmation        `json:"confirmation,omitempty"`
	CancellationDetails *CancellationDetails `json:"cancellation_details,omitempty"`
	Metadata            map[string]string    `json:"metadata,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	Test                bool                 `json:"test"`
}
