package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type config struct {
	httpAddr string
	dbDSN    string
	natsURL  string
	hmacKey  string
}

func loadConfig() (config, error) {
	cfg := config{
		httpAddr: envOrDefault("HTTP_ADDR", ":8080"),
		dbDSN:    os.Getenv("DB_DSN"),
		natsURL:  os.Getenv("NATS_URL"),
		hmacKey:  os.Getenv("HMAC_KEY"),
	}

	var missing []string
	if cfg.dbDSN == "" {
		missing = append(missing, "DB_DSN")
	}
	if cfg.natsURL == "" {
		missing = append(missing, "NATS_URL")
	}
	if cfg.hmacKey == "" {
		missing = append(missing, "HMAC_KEY")
	}
	if len(missing) > 0 {
		return config{}, fmt.Errorf("eksik ortam degiskenleri: %v", missing)
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("config yuklenemedi", "err", err)
		os.Exit(1)
	}

	srv := newServer(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("control-api dinliyor", "addr", cfg.httpAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server hata verdi", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("kapatma sinyali alindi, graceful shutdown baslatiliyor")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown basarisiz", "err", err)
		os.Exit(1)
	}

	logger.Info("control-api kapandi")
}

func newServer(cfg config) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)

	return &http.Server{
		Addr:              cfg.httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
