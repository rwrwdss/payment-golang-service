package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"

	"github.com/google/uuid"

	"payment-service/payments"
)

type yookassaNotification struct {
	Type   string `json:"type"`
	Event  string `json:"event"`
	Object struct {
		ID       string            `json:"id"`
		Metadata map[string]string `json:"metadata"`
	} `json:"object"`
}

var yookassaWebhookIPs = []netip.Prefix{
	netip.MustParsePrefix("185.71.76.0/27"),
	netip.MustParsePrefix("185.71.77.0/27"),
	netip.MustParsePrefix("77.75.153.0/25"),
	netip.MustParsePrefix("77.75.156.11/32"),
	netip.MustParsePrefix("77.75.156.35/32"),
	netip.MustParsePrefix("77.75.154.128/25"),
	netip.MustParsePrefix("2a02:5180::/32"),
}

func (h *handler) yookassaWebhook(w http.ResponseWriter, r *http.Request) {
	if !isYooKassaIP(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var notification yookassaNotification
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err := decoder.Decode(&notification); err != nil {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}

	if notification.Type != "notification" {
		http.Error(w, "invalid notification type", http.StatusBadRequest)
		return
	}
	if !isPaymentEvent(notification.Event) {
		log.Printf("yookassa webhook: ignored event %q", notification.Event)
		w.WriteHeader(http.StatusOK)
		return
	}
	if notification.Object.ID == "" {
		http.Error(w, "payment id is missing", http.StatusBadRequest)
		return
	}

	localID, err := uuid.Parse(notification.Object.Metadata["payment_id"])
	if err != nil {
		log.Printf("yookassa webhook: ignored payment %q without a valid local id", notification.Object.ID)
		w.WriteHeader(http.StatusOK)
		return
	}

	p, err := h.service.GetByID(r.Context(), localID)
	if errors.Is(err, payments.ErrNotFound) {
		log.Printf("yookassa webhook: ignored unknown local payment %s", localID)
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		log.Printf("yookassa webhook: load payment %s: %v", localID, err)
		http.Error(w, "temporary error", http.StatusServiceUnavailable)
		return
	}
	if p.ProviderID() != notification.Object.ID {
		log.Printf("yookassa webhook: ignored provider id mismatch for payment %s", localID)
		w.WriteHeader(http.StatusOK)
		return
	}

	p, err = h.service.Sync(r.Context(), localID)
	if err != nil {
		log.Printf("yookassa webhook: sync payment %s: %v", localID, err)
		http.Error(w, "temporary error", http.StatusServiceUnavailable)
		return
	}

	log.Printf("yookassa webhook: payment %s is %s", localID, p.Status())
	w.WriteHeader(http.StatusOK)
}

func isPaymentEvent(event string) bool {
	switch event {
	case "payment.waiting_for_capture", "payment.succeeded", "payment.canceled":
		return true
	default:
		return false
	}
}

// Проверяем адрес TCP-соединения, а не X-Forwarded-For: этот заголовок можно подделать.
// При размещении за прокси он должен проверять те же сети ЮKassa, а приложение —
// получать исходный адрес через явно доверенную настройку прокси.
func isYooKassaIP(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, prefix := range yookassaWebhookIPs {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
