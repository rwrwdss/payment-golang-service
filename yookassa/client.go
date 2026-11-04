// Package yookassa — клиент API ЮKassa v3 и адаптер к интерфейсу payments.Gateway.
//
// Документация: https://yookassa.ru/developers/api
package yookassa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// DefaultBaseURL — адрес боевого API. Тестовый магазин работает через него же,
	// отличаются только идентификатор магазина и ключ.
	DefaultBaseURL = "https://api.yookassa.ru/v3"

	defaultTimeout  = 10 * time.Second
	maxResponseBody = 1 << 20
)

// Config — настройки подключения к ЮKassa.
type Config struct {
	ShopID    string
	SecretKey string
	BaseURL   string        // если пусто, используется DefaultBaseURL
	Timeout   time.Duration // если 0, используется defaultTimeout
}

// Client выполняет запросы к API ЮKassa. Безопасен для использования из нескольких горутин.
type Client struct {
	baseURL   string
	shopID    string
	secretKey string
	http      *http.Client
}

// NewClient проверяет настройки и создаёт клиент.
func NewClient(cfg Config) (*Client, error) {
	if cfg.ShopID == "" || cfg.SecretKey == "" {
		return nil, errors.New("yookassa: shop id and secret key are required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}

	return &Client{
		baseURL:   cfg.BaseURL,
		shopID:    cfg.ShopID,
		secretKey: cfg.SecretKey,
		http:      &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// CreatePayment создаёт платёж. ЮKassa помнит idempotenceKey 24 часа:
// повторный запрос с тем же ключом вернёт уже созданный платёж, а не новый.
func (c *Client) CreatePayment(ctx context.Context, idempotenceKey string, req CreatePaymentRequest) (*Payment, error) {
	var p Payment
	if err := c.do(ctx, http.MethodPost, "/payments", idempotenceKey, req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPayment возвращает актуальное состояние платежа.
func (c *Client) GetPayment(ctx context.Context, id string) (*Payment, error) {
	var p Payment
	if err := c.do(ctx, http.MethodGet, "/payments/"+url.PathEscape(id), "", nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) do(ctx context.Context, method, path, idempotenceKey string, in, out any) error {
	req, err := c.newRequest(ctx, method, path, idempotenceKey, in)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("yookassa: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return decodeAPIError(resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(out); err != nil {
		return fmt.Errorf("yookassa: decode %s %s response: %w", method, path, err)
	}
	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path, idempotenceKey string, in any) (*http.Request, error) {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return nil, fmt.Errorf("yookassa: encode request: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("yookassa: build request: %w", err)
	}

	req.SetBasicAuth(c.shopID, c.secretKey)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotenceKey != "" {
		req.Header.Set("Idempotence-Key", idempotenceKey)
	}
	return req, nil
}
