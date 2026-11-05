// Package payments описывает платёж, правила смены его статусов
// и сценарии работы с платёжным провайдером.
package payments

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// maxDescriptionLen — ограничение ЮKassa на длину описания платежа (в символах).
const maxDescriptionLen = 128

// Payment — один платёж клиента.
// Поля приватные: читаются через геттеры, статус меняется только через методы переходов.
type Payment struct {
	id              uuid.UUID // наш идентификатор, он же ключ идемпотентности для провайдера
	amount          int64     // сумма в минимальных единицах валюты (копейки, центы)
	currency        Currency  // валюта платежа
	method          Method    // способ оплаты, задаётся только в конструкторе
	description     string    // описание, которое клиент увидит на странице оплаты
	status          Status    // текущий этап платежа
	providerID      string    // идентификатор платежа в ЮKassa, появляется после Submit
	confirmationURL string    // страница оплаты, куда нужно отправить клиента
	cancelReason    string    // причина отмены от провайдера, например insufficient_funds
	createdAt       time.Time // когда платёж создан
	updatedAt       time.Time // когда статус менялся последний раз
}

// PaymentRecord — данные платежа для сохранения в постоянном хранилище.
type PaymentRecord struct {
	ID              uuid.UUID
	Amount          int64
	Currency        Currency
	Method          Method
	Description     string
	Status          Status
	ProviderID      string
	ConfirmationURL string
	CancelReason    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewPayment проверяет входные данные и создаёт платёж в статусе StatusNew.
func NewPayment(amount int64, currency Currency, method Method, description string) (*Payment, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	if !currency.IsValid() {
		return nil, ErrInvalidCurrency
	}

	if !method.IsValid() {
		return nil, ErrInvalidMethod
	}

	if utf8.RuneCountInString(description) > maxDescriptionLen {
		return nil, ErrInvalidDescription
	}

	now := time.Now()

	return &Payment{
		id:          uuid.New(),
		amount:      amount,
		currency:    currency,
		method:      method,
		description: description,
		status:      StatusNew,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// ID возвращает уникальный идентификатор платежа.
func (p *Payment) ID() uuid.UUID { return p.id }

// Amount возвращает сумму в минимальных единицах валюты.
func (p *Payment) Amount() int64 { return p.amount }

// Currency возвращает валюту платежа.
func (p *Payment) Currency() Currency { return p.currency }

// Method возвращает способ оплаты платежа.
func (p *Payment) Method() Method { return p.method }

// Description возвращает описание платежа.
func (p *Payment) Description() string { return p.description }

// Status возвращает текущий статус платежа.
func (p *Payment) Status() Status { return p.status }

// ProviderID возвращает идентификатор платежа в ЮKassa или пустую строку, если платёж ещё не отправлен.
func (p *Payment) ProviderID() string { return p.providerID }

// ConfirmationURL возвращает ссылку на страницу оплаты.
func (p *Payment) ConfirmationURL() string { return p.confirmationURL }

// CancelReason возвращает причину отмены, если платёж отменён.
func (p *Payment) CancelReason() string { return p.cancelReason }

// CreatedAt возвращает время создания платежа.
func (p *Payment) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt возвращает время последней смены статуса.
func (p *Payment) UpdatedAt() time.Time { return p.updatedAt }

// Record возвращает копию данных платежа для репозитория.
func (p *Payment) Record() PaymentRecord {
	return PaymentRecord{
		ID:              p.id,
		Amount:          p.amount,
		Currency:        p.currency,
		Method:          p.method,
		Description:     p.description,
		Status:          p.status,
		ProviderID:      p.providerID,
		ConfirmationURL: p.confirmationURL,
		CancelReason:    p.cancelReason,
		CreatedAt:       p.createdAt,
		UpdatedAt:       p.updatedAt,
	}
}

// RestorePayment восстанавливает платёж из записи постоянного хранилища.
func RestorePayment(record PaymentRecord) (*Payment, error) {
	if record.ID == uuid.Nil || record.Amount <= 0 || !record.Currency.IsValid() || !record.Method.IsValid() ||
		utf8.RuneCountInString(record.Description) > maxDescriptionLen || !record.Status.IsValid() ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		((record.Status == StatusPending || record.Status == StatusSucceeded) && record.ProviderID == "") {
		return nil, ErrInvalidRecord
	}

	return &Payment{
		id:              record.ID,
		amount:          record.Amount,
		currency:        record.Currency,
		method:          record.Method,
		description:     record.Description,
		status:          record.Status,
		providerID:      record.ProviderID,
		confirmationURL: record.ConfirmationURL,
		cancelReason:    record.CancelReason,
		createdAt:       record.CreatedAt,
		updatedAt:       record.UpdatedAt,
	}, nil
}

// Submit отмечает, что платёж создан у провайдера и ждёт оплаты клиентом.
func (p *Payment) Submit(providerID, confirmationURL string) error {
	if providerID == "" {
		return ErrMissingProviderID
	}
	if err := p.transitionTo(StatusPending); err != nil {
		return err
	}

	p.providerID = providerID
	p.confirmationURL = confirmationURL
	return nil
}

// Succeed отмечает, что провайдер подтвердил списание.
func (p *Payment) Succeed() error { return p.transitionTo(StatusSucceeded) }

// Cancel отмечает, что платёж отменён, и запоминает причину.
func (p *Payment) Cancel(reason string) error {
	if err := p.transitionTo(StatusCanceled); err != nil {
		return err
	}

	p.cancelReason = reason
	return nil
}

// transitionTo — единственное место, где меняется статус.
// При запрещённом переходе платёж остаётся без изменений.
func (p *Payment) transitionTo(next Status) error {
	if !p.status.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, p.status, next)
	}

	p.status = next
	p.updatedAt = time.Now()
	return nil
}
