// Command sensor-ssh, SSH auth log'unu takip eden deception sensorudur (APP-4/5).
// Her parola denemesini Decode eder; deneme bilinen bir canary kullanicisiyla
// yapilmissa bir TripEvent'i trip.events.v1'e (NATS) publish eder. Canary
// olmayan girisler hicbir sey uretmez (sifir-FP).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/sensorssh"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

const (
	sensorName          = "sensor-ssh"
	canaryRefreshPeriod = 30 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	authLogPath := envOrDefault("AUTH_LOG_PATH", "/var/log/auth.log")
	natsURL := envOrDefault("NATS_URL", nats.DefaultURL)
	controlAPIURL := envOrDefault("CONTROL_API_URL", "http://control-api:8080")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	nc, err := nats.Connect(natsURL, nats.Name(sensorName))
	if err != nil {
		logger.Error("nats'a baglanilamadi", "err", err)
		os.Exit(1)
	}
	defer nc.Close()

	resolver := sensorssh.NewCanaryResolver(controlAPIURL, logger)
	if err := resolver.Refresh(ctx); err != nil {
		logger.Warn("ilk canary listesi alinamadi, arka planda yeniden denenecek", "err", err)
	}
	go resolver.Run(ctx, canaryRefreshPeriod)

	decoder := sensorssh.NewDecoder(resolver, sensorName)

	logger.Info("sensor-ssh basladi", "auth_log", authLogPath, "control_api", controlAPIURL)

	handle := func(line string) {
		obs := trap.RawObservation{
			Sensor:     sensorName,
			Line:       line,
			ObservedAt: time.Now().UTC(),
		}
		ev, err := decoder.Decode(obs)
		if errors.Is(err, trap.ErrNotATrip) {
			return // canary degil: sifir-FP, hicbir sey yayinlanmaz
		}
		if err != nil {
			logger.Error("gozlem decode edilemedi", "err", err)
			return
		}

		payload, err := json.Marshal(ev)
		if err != nil {
			logger.Error("tripevent serilestirilemedi", "err", err)
			return
		}
		if err := nc.Publish(trap.SubjectTripEvents, payload); err != nil {
			logger.Error("tripevent publish edilemedi", "err", err)
			return
		}
		logger.Info("trip yayinlandi",
			"event_id", ev.EventID, "trap_id", ev.TrapID, "source", ev.Source)
	}

	if err := sensorssh.TailFile(ctx, authLogPath, 0, false, handle); err != nil {
		logger.Error("auth log takibi hata verdi", "err", err)
		os.Exit(1)
	}

	// Kapanmadan once tamponlanmis publish'leri bosalt.
	if err := nc.Flush(); err != nil {
		logger.Error("nats flush hata verdi", "err", err)
	}
	logger.Info("sensor-ssh kapandi")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
