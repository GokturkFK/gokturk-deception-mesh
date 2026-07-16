// Package alerting, APP-6'nin persist ettigi trip'leri korelasyona sokup
// sonucu kalici katmana yazan entegrasyon katmanidir (APP-7). Korelasyon
// algoritmasi internal/correlate'te, kalicilik internal/store'dadir; bu
// paket yalnizca ikisini birbirine baglar ve pencere secimini yapar
// ("zaman penceresini secmek ... caginanin sorumlulugudur", correlate.Evaluate).
package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// DefaultWindow, aksi belirtilmedigi surece kullanilan korelasyon
// penceresidir (PLAN APP-7: "son N dakikanin trip'leri").
const DefaultWindow = 15 * time.Minute

// TripLister, korelasyon penceresindeki trip'leri saglayan sozlesmedir.
type TripLister interface {
	ListTripEventsSince(ctx context.Context, source string, since time.Time) ([]trap.TripEvent, error)
}

// AlertUpserter, korelasyon sonucunu kalici katmana yazan sozlesmedir.
type AlertUpserter interface {
	UpsertAlert(ctx context.Context, a correlate.Alert) (correlate.Alert, error)
}

// Engine, bir kaynaktaki yeni trip'i mevcut pencereyle yeniden
// degerlendirip tek bir alarma indirger.
type Engine struct {
	trips  TripLister
	alerts AlertUpserter
	window time.Duration
	logger *slog.Logger
}

// New, verilen bagimliliklarla bir Engine olusturur. window <= 0 ise
// DefaultWindow kullanilir.
func New(trips TripLister, alerts AlertUpserter, window time.Duration, logger *slog.Logger) *Engine {
	if window <= 0 {
		window = DefaultWindow
	}
	return &Engine{trips: trips, alerts: alerts, window: window, logger: logger}
}

// Correlate, verilen kaynagin son pencere icindeki tum trip'lerini yeniden
// degerlendirir ve sonucu upsert eder (PLAN APP-7 AC: tek trip -> High,
// ayni kaynaktan >=2 trip -> tek Critical).
func (e *Engine) Correlate(ctx context.Context, source string) error {
	since := time.Now().Add(-e.window)
	trips, err := e.trips.ListTripEventsSince(ctx, source, since)
	if err != nil {
		return fmt.Errorf("alerting: trip'ler alinamadi: %w", err)
	}
	if len(trips) == 0 {
		return nil
	}

	// Evaluate girdiler tek kaynaktan oldugu icin en fazla 1 alarm doner.
	for _, a := range correlate.Evaluate(trips) {
		saved, err := e.alerts.UpsertAlert(ctx, a)
		if err != nil {
			return fmt.Errorf("alerting: alarm yazilamadi: %w", err)
		}
		e.logger.Info("alarm guncellendi",
			"alert_id", saved.ID, "source", saved.Source,
			"severity", saved.Severity, "trip_count", saved.TripCount)
	}
	return nil
}
