// Package ingest, sensorlerin trip.events.v1 subject'ine yayinladigi
// TripEvent'leri tuketip kalici katmana idempotent olarak yazar (APP-6).
// Korelasyon bu paketin isi degildir; persist edilen trip'ler uzerinden
// ayrica calisir (APP-7).
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// StreamName, trip event'lerini kalici tutan JetStream stream'idir.
const StreamName = "TRIPS"

// DurableName, control-api'nin durable consumer adidir; yeniden baslatmada
// kaldigi yerden devam etmesini saglar.
const DurableName = "control-api-ingest"

// Store, consumer'in ihtiyac duydugu kalici katman sozlesmesidir.
// bool donusu: true = yeni kayit, false = ayni event_id zaten vardi.
type Store interface {
	InsertTripEvent(ctx context.Context, ev trap.TripEvent) (bool, error)
}

// Consumer, tek tek TripEvent mesajlarini isler.
type Consumer struct {
	store  Store
	logger *slog.Logger

	// OnInserted, yeni (yinelenmeyen) bir trip basariyla yazildiktan sonra
	// cagirilir. Korelasyonu tetiklemek icin control-api tarafindan
	// baglanir (APP-7); nil birakilabilir. Hatasi Handle'i basarisiz
	// kilmaz — trip zaten kalicidir, sadece loglanir.
	OnInserted func(ctx context.Context, ev trap.TripEvent) error
}

// New, verilen store ve logger ile bir Consumer olusturur.
func New(store Store, logger *slog.Logger) *Consumer {
	return &Consumer{store: store, logger: logger}
}

// Handle, tek bir mesaj govdesini isler. Gecersiz mesajlar (bozuk JSON,
// eksik alan) hata dondurur; cagiran loglayip mesaji yine de ack'lemelidir —
// poison mesaj yeniden teslim dongusune girmemeli. Yinelenen event_id hata
// degildir (idempotentlik, PLAN APP-6 AC).
func (c *Consumer) Handle(ctx context.Context, data []byte) error {
	var ev trap.TripEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("ingest: mesaj cozulemedi: %w", err)
	}
	if err := validate(ev); err != nil {
		return err
	}

	inserted, err := c.store.InsertTripEvent(ctx, ev)
	if err != nil {
		return fmt.Errorf("ingest: trip yazilamadi: %w", err)
	}
	if !inserted {
		c.logger.Debug("yinelenen trip atlandi", "event_id", ev.EventID)
		return nil
	}

	c.logger.Info("trip kaydedildi",
		"event_id", ev.EventID, "trap_id", ev.TrapID,
		"sensor", ev.Sensor, "source", ev.Source)

	if c.OnInserted != nil {
		if err := c.OnInserted(ctx, ev); err != nil {
			c.logger.Error("trip sonrasi kanca basarisiz", "event_id", ev.EventID, "err", err)
		}
	}
	return nil
}

func validate(ev trap.TripEvent) error {
	var missing []string
	if ev.EventID == "" {
		missing = append(missing, "event_id")
	}
	if ev.TrapID == "" {
		missing = append(missing, "trap_id")
	}
	if ev.Sensor == "" {
		missing = append(missing, "sensor")
	}
	if ev.Source == "" {
		missing = append(missing, "source")
	}
	if ev.ObservedAt.IsZero() {
		missing = append(missing, "observed_at")
	}
	if len(missing) > 0 {
		return fmt.Errorf("ingest: eksik alanlar: %v", missing)
	}
	return nil
}

// Run, TRIPS stream'ini garanti eder, durable consumer uzerinden mesaj
// tuketir ve ctx iptal edilene kadar bloklar. Sensor core NATS ile
// yayinlasa da subject stream'e bagli oldugu icin mesajlar kalicidir.
func Run(ctx context.Context, nc *nats.Conn, c *Consumer) error {
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("ingest: jetstream baglami olusturulamadi: %w", err)
	}

	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamName,
		Subjects: []string{trap.SubjectTripEvents},
		Storage:  jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("ingest: stream olusturulamadi: %w", err)
	}

	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:   DurableName,
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("ingest: consumer olusturulamadi: %w", err)
	}

	cc, err := cons.Consume(func(msg jetstream.Msg) {
		if err := c.Handle(ctx, msg.Data()); err != nil {
			c.logger.Error("trip islenemedi", "err", err)
		}
		// Gecersiz mesaj da ack'lenir: yeniden teslim onu gecerli yapmaz.
		if err := msg.Ack(); err != nil {
			c.logger.Error("mesaj ack'lenemedi", "err", err)
		}
	})
	if err != nil {
		return fmt.Errorf("ingest: tuketim baslatilamadi: %w", err)
	}
	defer cc.Stop()

	<-ctx.Done()
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
