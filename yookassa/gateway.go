package yookassa

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"payment-service/payments"
)

var _ payments.Gateway = (*Gateway)(nil)

// methodTypes переводит наш способ оплаты в payment_method_data.type ЮKassa.
var methodTypes = map[payments.Method]string{
	payments.MethodCard: "bank_card",
	payments.MethodSBP:  "sbp",
}

// Gateway подключает ЮKassa к сервису платежей, реализует payments.Gateway.
type Gateway struct {
	client    *Client
	returnURL string
}

// NewGateway создаёт адаптер. returnURL — страница магазина, куда ЮKassa вернёт клиента после оплаты.
func NewGateway(client *Client, returnURL string) *Gateway {
	return &Gateway{client: client, returnURL: returnURL}
}

// CreatePayment создаёт одностадийный платёж (capture=true): деньги списываются сразу после подтверждения.
// Наш ID платежа уходит и ключом идемпотентности, и в metadata, чтобы по вебхуку найти платёж у себя.
func (g *Gateway) CreatePayment(ctx context.Context, p *payments.Payment) (payments.Checkout, error) {
	req := CreatePaymentRequest{
		Amount:            Amount{Value: formatAmount(p.Amount()), Currency: string(p.Currency())},
		Capture:           true,
		Confirmation:      Confirmation{Type: ConfirmationRedirect, ReturnURL: g.returnURL},
		PaymentMethodData: &PaymentMethodData{Type: methodTypes[p.Method()]},
		Description:       p.Description(),
		Metadata:          map[string]string{"payment_id": p.ID().String()},
	}

	created, err := g.client.CreatePayment(ctx, p.ID().String(), req)
	if err != nil {
		return payments.Checkout{}, classifyCreateError(err)
	}

	checkout := payments.Checkout{ProviderID: created.ID}
	if created.Confirmation != nil {
		checkout.ConfirmationURL = created.Confirmation.ConfirmationURL
	}
	return checkout, nil
}

// FetchPayment запрашивает платёж в ЮKassa и переводит его статус в наш.
func (g *Gateway) FetchPayment(ctx context.Context, providerID string) (payments.ProviderState, error) {
	p, err := g.client.GetPayment(ctx, providerID)
	if err != nil {
		return payments.ProviderState{}, err
	}

	switch p.Status {
	case StatusPending, StatusWaitingForCapture:
		// waiting_for_capture бывает только у двухстадийных платежей, мы их не создаём.
		// Если всё же пришёл, считаем платёж незавершённым: ЮKassa сама отменит его по истечении срока.
		return payments.ProviderState{Status: payments.StatusPending}, nil
	case StatusSucceeded:
		return payments.ProviderState{Status: payments.StatusSucceeded}, nil
	case StatusCanceled:
		return payments.ProviderState{Status: payments.StatusCanceled, CancelReason: cancelReason(p)}, nil
	}
	return payments.ProviderState{}, fmt.Errorf("yookassa: payment %s has unknown status %q", p.ID, p.Status)
}

// classifyCreateError помечает окончательные отказы.
// 400 означает, что сам запрос неверен, и повтор ничего не изменит.
// 401 и 403 — проблема наших настроек, а не платежа: отменять его из-за них нельзя.
func classifyCreateError(err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("%w: %w", payments.ErrPaymentRejected, err)
	}
	return err
}

func cancelReason(p *Payment) string {
	if p.CancellationDetails == nil {
		return ""
	}
	return p.CancellationDetails.Reason
}

// formatAmount переводит копейки в формат ЮKassa: 15000 -> "150.00".
// У всех поддерживаемых валют (RUB, USD, EUR) два знака после точки.
func formatAmount(minor int64) string {
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}
