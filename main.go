package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"payment-service/httpapi"
	"payment-service/payments"
	"payment-service/storage/postgres"
	"payment-service/yookassa"
)

func main() {
	if err := run(); err != nil {
		log.Printf("payment API: %v", err)
		os.Exit(1)
	}
}

func run() error {
	client, err := yookassa.NewClient(yookassa.Config{
		ShopID:    os.Getenv("YOOKASSA_SHOP_ID"),
		SecretKey: os.Getenv("YOOKASSA_SECRET_KEY"),
	})
	if err != nil {
		return fmt.Errorf("config: %w (set YOOKASSA_SHOP_ID and YOOKASSA_SECRET_KEY)", err)
	}

	returnURL := os.Getenv("YOOKASSA_RETURN_URL")
	if returnURL == "" {
		return errors.New("config: set YOOKASSA_RETURN_URL")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("config: set DATABASE_URL")
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := db.PingContext(startupCtx); err != nil {
		cancel()
		return fmt.Errorf("connect to database: %w", err)
	}
	if err := postgres.EnsureSchema(startupCtx, db); err != nil {
		cancel()
		return fmt.Errorf("prepare database: %w", err)
	}
	cancel()

	svc := payments.NewService(postgres.NewPaymentRepository(db), yookassa.NewGateway(client, returnURL))
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.Handle("/", httpapi.Handler(svc))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("payment API listening on %s", addr)
	return serve(ctx, server, 10*time.Second)
}

func serve(ctx context.Context, server *http.Server, shutdownTimeout time.Duration) error {
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	var closeErr error
	if shutdownErr != nil {
		closeErr = server.Close()
	}

	serveErr := <-serverErrors
	if shutdownErr != nil {
		if closeErr != nil {
			return fmt.Errorf("shutdown server: %w (close server: %v)", shutdownErr, closeErr)
		}
		return fmt.Errorf("shutdown server: %w", shutdownErr)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", serveErr)
	}
	return nil
}
