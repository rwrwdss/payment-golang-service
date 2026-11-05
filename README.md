# payment-gateway

![payment-gateway на Go и ЮKassa](images/logo-baner.png)

Небольшой учебный сервис на Go. Он создаёт платёж в ЮKassa, возвращает ссылку для оплаты и обновляет статус по webhook.

## Что умеет

- Создавать платёж через `POST /payments`.
- Возвращать платёж по ID через `GET /payments/{id}`.
- Принимать уведомления ЮKassa на `POST /webhooks/yookassa` и сверять статус через API провайдера.

Пока платежи хранятся в памяти процесса. После перезапуска сервера они исчезнут.

## Запуск

Нужен Go 1.25.7 и тестовый магазин ЮKassa. Укажите настройки в переменных окружения:

```sh
export YOOKASSA_SHOP_ID=your_shop_id
export YOOKASSA_SECRET_KEY=your_secret_key
export YOOKASSA_RETURN_URL=https://example.com/payment-return
export LISTEN_ADDR=:8080

go run .
```

`LISTEN_ADDR` можно не задавать, по умолчанию сервер слушает порт 8080.

## Создать платёж

Сумма передаётся в копейках. Например, `15000` означает 150 рублей.

```sh
curl -X POST http://localhost:8080/payments \
  -H 'Content-Type: application/json' \
  -d '{"amount":15000,"currency":"RUB","method":"card","description":"Заказ 1"}'
```

В ответе будут ID и статус платежа, а после успешного создания у провайдера появится `confirmation_url`. Откройте эту ссылку, чтобы перейти к оплате.

Проверить статус можно так:

```sh
curl http://localhost:8080/payments/123e4567-e89b-12d3-a456-426614174000
```

## Webhook

В кабинете ЮKassa укажите публичный HTTPS адрес `https://ваш-домен/webhooks/yookassa` и события `payment.succeeded`, `payment.canceled` и `payment.waiting_for_capture`. Сервис проверяет IP отправителя и запрашивает текущий статус у ЮKassa.

Запустить проверки проекта локально:

```sh
go test ./...
```

GitHub Actions выполняет эту команду при каждом push и pull request.
