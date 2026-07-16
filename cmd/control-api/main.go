package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/store"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// hmacKeyMinLen, canary secret'lerini imzalayan HMAC anahtarinin asgari
// uzunlugudur (bkz. deployments/docker/.env.example: "en az 32 byte").
const hmacKeyMinLen = 32

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

	// APP-2 ile birlikte HMAC_KEY artik secret imzalamada kullaniliyor;
	// zayif anahtari erkenden reddet.
	if len(cfg.hmacKey) < hmacKeyMinLen {
		return config{}, fmt.Errorf("HMAC_KEY en az %d bayt olmali (mevcut %d)", hmacKeyMinLen, len(cfg.hmacKey))
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
	// Container HEALTHCHECK modu: distroless image'da wget/curl olmadigi icin
	// ayni binary "healthcheck" arguman ile calistirilir (bkz. control-api.Dockerfile).
	// Sadece HTTP_ADDR'a ihtiyac duyar, diger env zorunluluklarindan muaf.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck(envOrDefault("HTTP_ADDR", ":8080")))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("config yuklenemedi", "err", err)
		os.Exit(1)
	}

	db, err := sql.Open("postgres", cfg.dbDSN)
	if err != nil {
		logger.Error("postgres surucusu acilamadi", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	pingCtx, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		cancelPing()
		logger.Error("postgres'e baglanilamadi", "err", err)
		os.Exit(1)
	}
	cancelPing()

	api := &apiServer{
		provider: trap.NewCredentialCanaryProvider([]byte(cfg.hmacKey)),
		store:    store.New(db),
		logger:   logger,
	}
	srv := newServer(cfg, api)

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

func newServer(cfg config, api *apiServer) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("POST /api/v1/traps", api.handleCreateTrap)
	mux.HandleFunc("GET /api/v1/traps", api.handleListTraps)

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

// healthURL, ":8080" veya "0.0.0.0:8080" gibi bir listen adresinden
// loopback uzerinden erisilebilir healthz URL'i uretir.
func healthURL(addr string) string {
	port := addr[strings.LastIndex(addr, ":")+1:]
	return "http://127.0.0.1:" + port + "/healthz"
}

func runHealthcheck(addr string) int {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, healthURL(addr), nil)
	if err != nil {
		return 1
	}
	resp, err := client.Do(req)
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
