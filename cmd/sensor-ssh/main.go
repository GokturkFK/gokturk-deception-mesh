// Command sensor-ssh, SSH auth log'unu takip eden deception sensorudur (APP-4).
// Su an her parola denemesinden kullanici + kaynak IP cikarir ve loglar; APP-5'te
// bu gozlemler Decode edilip trip ise trip.events.v1'e publish edilecek.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/sensorssh"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

const sensorName = "sensor-ssh"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	authLogPath := envOrDefault("AUTH_LOG_PATH", "/var/log/auth.log")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("sensor-ssh basladi", "auth_log", authLogPath)

	handle := func(line string) {
		ev, ok := sensorssh.ParseAuthLine(line)
		if !ok {
			return
		}
		obs := trap.RawObservation{
			Sensor:     sensorName,
			Source:     ev.Source,
			Line:       line,
			ObservedAt: time.Now().UTC(),
		}
		// APP-5: burada Decode + trip ise NATS publish yapilacak.
		logger.Info("ssh parola denemesi gozlemlendi",
			"username", ev.Username,
			"source", obs.Source,
			"accepted", ev.Accepted,
		)
	}

	if err := sensorssh.TailFile(ctx, authLogPath, 0, false, handle); err != nil {
		logger.Error("auth log takibi hata verdi", "err", err)
		os.Exit(1)
	}

	logger.Info("sensor-ssh kapandi")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
