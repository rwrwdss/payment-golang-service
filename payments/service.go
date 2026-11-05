package payments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Repository хранит платежи и атомарно связывает их с ключом запроса.
// GetByID должен возвращать копию: изменения попадают в хранилище через Save.
type Repository interface {
	Save(ctx context.Context, p *Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Payment, error)
	CreateWithIdempotency(ctx context.Context, p *Payment, key, requestHash string) (*Payment, error)
}

// Gateway — платёжный провайдер.
type Gateway interface {
	// CreatePayment создаёт платёж у провайдера. Повторный вызов для того же платежа
	// не создаёт второй: p.ID() передаётся провайдеру как ключ идемпотентности.
	// ErrPaymentRejected означает окончательный отказ, любая другая ошибка — временную проблему.
	CreatePayment(ctx context.Context, p *Payment) (Checkout, error)

	// FetchPayment возвращает актуальное состояние платежа у провайдера.
	FetchPayment(ctx context.Context, providerID string) (ProviderState, error)
}

// Checkout — ответ провайдера на создание платежа.
type Checkout struct {
	ProviderID      string
	ConfirmationURL string
}

// ProviderState — состояние платежа на стороне провайдера.
type ProviderState struct {
	Status       Status
	CancelReason string
}

// Service — сценарии работы с платежами: создание, повторная отправка и сверка статуса с провайдером.
type Service struct {
	repo    Repository
	gateway Gateway
}

// NewService собирает сервис из хранилища и провайдера.
func NewService(repo Repository, gateway Gateway) *Service {
	return &Service{repo: repo, gateway: gateway}
}

// Create сохраняет новый платёж и отправляет его провайдеру.
//
// Если провайдер недоступен, платёж остаётся в StatusNew и возвращается вместе с ошибкой:
// его можно безопасно отправить ещё раз через Submit.
// Если провайдер отказал окончательно, платёж возвращается в StatusCanceled без ошибки.
func (s *Service) Create(ctx context.Context, amount int64, currency Currency, method Method, description string) (*Payment, error) {
	return s.create(ctx, "", amount, currency, method, description)
}

// CreateWithIdempotencyKey создаёт платёж один раз для заданного ключа.
// Повтор с тем же ключом и данными возвращает исходный платёж.
func (s *Service) CreateWithIdempotencyKey(ctx context.Context, key string, amount int64, currency Currency, method Method, description string) (*Payment, error) {
	if len(key) == 0 || len(key) > 128 {
		return nil, ErrInvalidIdempotencyKey
	}
	return s.create(ctx, key, amount, currency, method, description)
}

func (s *Service) create(ctx context.Context, key string, amount int64, currency Currency, method Method, description string) (*Payment, error) {
	p, err := NewPayment(amount, currency, method, description)
	if err != nil {
		return nil, err
	}
	newPaymentID := p.ID()

	if key == "" {
		if err := s.repo.Save(ctx, p); err != nil {
			return nil, fmt.Errorf("save payment %s: %w", newPaymentID, err)
		}
	} else {
		requestHash, err := hashCreateRequest(amount, currency, method, description)
		if err != nil {
			return nil, fmt.Errorf("hash create request: %w", err)
		}
		p, err = s.repo.CreateWithIdempotency(ctx, p, key, requestHash)
		if err != nil {
			return nil, fmt.Errorf("save payment %s: %w", newPaymentID, err)
		}
	}

	return s.submit(ctx, p)
}

func hashCreateRequest(amount int64, currency Currency, method Method, description string) (string, error) {
	request := struct {
		Amount      int64    `json:"amount"`
		Currency    Currency `json:"currency"`
		Method      Method   `json:"method"`
		Description string   `json:"description"`
	}{amount, currency, method, description}

	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:]), nil
}

// Submit повторно отправляет провайдеру платёж, который застрял в StatusNew.
// Для уже отправленных платежей ничего не делает.
func (s *Service) Submit(ctx context.Context, id uuid.UUID) (*Payment, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.submit(ctx, p)
}

// GetByID возвращает сохранённый платёж.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Payment, error) {
	return s.repo.GetByID(ctx, id)
}

// Sync сверяет статус платежа с провайдером и сохраняет изменения.
// Вызывается по вебхуку: данным из уведомления не доверяем и всегда спрашиваем провайдера сами.
// Повторный вызов безопасен, финальные платежи провайдеру не переспрашиваются.
func (s *Service) Sync(ctx context.Context, id uuid.UUID) (*Payment, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Status() != StatusPending {
		return p, nil
	}

	state, err := s.gateway.FetchPayment(ctx, p.ProviderID())
	if err != nil {
		return nil, fmt.Errorf("fetch payment %s from provider: %w", p.ID(), err)
	}
	if state.Status == StatusPending {
		return p, nil
	}
	if err := applyProviderState(p, state); err != nil {
		return nil, fmt.Errorf("sync payment %s: %w", p.ID(), err)
	}

	return p, s.save(ctx, p)
}

func (s *Service) submit(ctx context.Context, p *Payment) (*Payment, error) {
	if p.Status() != StatusNew {
		return p, nil
	}

	checkout, err := s.gateway.CreatePayment(ctx, p)
	switch {
	case errors.Is(err, ErrPaymentRejected):
		err = p.Cancel(err.Error())
	case err != nil:
		return p, fmt.Errorf("create payment %s at provider: %w", p.ID(), err)
	default:
		err = p.Submit(checkout.ProviderID, checkout.ConfirmationURL)
	}
	if err != nil {
		return p, fmt.Errorf("submit payment %s: %w", p.ID(), err)
	}

	return p, s.save(ctx, p)
}

func (s *Service) save(ctx context.Context, p *Payment) error {
	if err := s.repo.Save(ctx, p); err != nil {
		return fmt.Errorf("save payment %s: %w", p.ID(), err)
	}
	return nil
}

func applyProviderState(p *Payment, state ProviderState) error {
	switch state.Status {
	case StatusSucceeded:
		return p.Succeed()
	case StatusCanceled:
		return p.Cancel(state.CancelReason)
	}
	return fmt.Errorf("unexpected provider status %q", state.Status)
}
