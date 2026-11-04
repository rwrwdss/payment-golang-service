package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"payment-service/httpapi"
	"payment-service/payments"
	"payment-service/storage/memory"
	"payment-service/yookassa"
)

func main() {
	client, err := yookassa.NewClient(yookassa.Config{
		ShopID:    os.Getenv("YOOKASSA_SHOP_ID"),
		SecretKey: os.Getenv("YOOKASSA_SECRET_KEY"),
	})
	if err != nil {
		log.Fatalf("config: %v (set YOOKASSA_SHOP_ID and YOOKASSA_SECRET_KEY)", err)
	}

	returnURL := os.Getenv("YOOKASSA_RETURN_URL")
	if returnURL == "" {
		log.Fatal("config: set YOOKASSA_RETURN_URL")
	}

	svc := payments.NewService(memory.NewPaymentRepository(), yookassa.NewGateway(client, returnURL))
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.Handler(svc),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown server: %v", err)
		}
	}()

	log.Printf("payment API listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve HTTP: %v", err)
	}
}
