package alerting

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

type fakeTrips struct {
	events []trap.TripEvent
	err    error
}

func (f *fakeTrips) ListTripEventsSince(_ context.Context, source string, _ time.Time) ([]trap.TripEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []trap.TripEvent
	for _, ev := range f.events {
		if ev.Source == source {
			out = append(out, ev)
		}
	}
	return out, nil
}

type fakeAlerts struct {
	upserted []correlate.Alert
	err      error
}

func (f *fakeAlerts) UpsertAlert(_ context.Context, a correlate.Alert) (correlate.Alert, error) {
	if f.err != nil {
		return correlate.Alert{}, f.err
	}
	a.ID = "alert-1"
	f.upserted = append(f.upserted, a)
	return a, nil
}

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestCorrelate_SingleTripIsHigh(t *testing.T) {
	trips := &fakeTrips{events: []trap.TripEvent{
		{EventID: "e1", Source: "10.0.0.1", ObservedAt: time.Now().UTC()},
	}}
	alerts := &fakeAlerts{}
	e := New(trips, alerts, 0, testLogger())

	if err := e.Correlate(context.Background(), "10.0.0.1"); err != nil {
		t.Fatalf("Correlate: %v", err)
	}
	if len(alerts.upserted) != 1 {
		t.Fatalf("upsert sayisi = %d, istenen 1", len(alerts.upserted))
	}
	if alerts.upserted[0].Severity != correlate.SeverityHigh {
		t.Errorf("severity = %q, istenen High", alerts.upserted[0].Severity)
	}
}

func TestCorrelate_TwoTripsIsSingleCriticalAlert(t *testing.T) {
	now := time.Now().UTC()
	trips := &fakeTrips{events: []trap.TripEvent{
		{EventID: "e1", Source: "10.0.0.1", ObservedAt: now.Add(-1 * time.Minute)},
		{EventID: "e2", Source: "10.0.0.1", ObservedAt: now},
	}}
	alerts := &fakeAlerts{}
	e := New(trips, alerts, 0, testLogger())

	if err := e.Correlate(context.Background(), "10.0.0.1"); err != nil {
		t.Fatalf("Correlate: %v", err)
	}
	if len(alerts.upserted) != 1 {
		t.Fatalf("upsert sayisi = %d, istenen 1 (kampanya birlesmesi)", len(alerts.upserted))
	}
	got := alerts.upserted[0]
	if got.Severity != correlate.SeverityCritical {
		t.Errorf("severity = %q, istenen Critical", got.Severity)
	}
	if got.TripCount != 2 {
		t.Errorf("trip_count = %d, istenen 2", got.TripCount)
	}
}

func TestCorrelate_NoTripsNoUpsert(t *testing.T) {
	e := New(&fakeTrips{}, &fakeAlerts{}, 0, testLogger())

	if err := e.Correlate(context.Background(), "10.0.0.1"); err != nil {
		t.Fatalf("Correlate: %v", err)
	}
}

func TestCorrelate_ListError(t *testing.T) {
	e := New(&fakeTrips{err: errors.New("db koptu")}, &fakeAlerts{}, 0, testLogger())

	if err := e.Correlate(context.Background(), "10.0.0.1"); err == nil {
		t.Fatal("liste hatasi Correlate'ten donmeliydi")
	}
}

func TestCorrelate_UpsertError(t *testing.T) {
	trips := &fakeTrips{events: []trap.TripEvent{
		{EventID: "e1", Source: "10.0.0.1", ObservedAt: time.Now().UTC()},
	}}
	e := New(trips, &fakeAlerts{err: errors.New("db koptu")}, 0, testLogger())

	if err := e.Correlate(context.Background(), "10.0.0.1"); err == nil {
		t.Fatal("upsert hatasi Correlate'ten donmeliydi")
	}
}

func TestNew_ZeroWindowUsesDefault(t *testing.T) {
	e := New(&fakeTrips{}, &fakeAlerts{}, 0, testLogger())
	if e.window != DefaultWindow {
		t.Errorf("window = %v, istenen DefaultWindow (%v)", e.window, DefaultWindow)
	}
}

func TestCorrelate_OnlySameSourceTripsCounted(t *testing.T) {
	now := time.Now().UTC()
	trips := &fakeTrips{events: []trap.TripEvent{
		{EventID: "e1", Source: "10.0.0.1", ObservedAt: now},
		{EventID: "e2", Source: "10.0.0.2", ObservedAt: now}, // farkli kaynak
	}}
	alerts := &fakeAlerts{}
	e := New(trips, alerts, 0, testLogger())

	if err := e.Correlate(context.Background(), "10.0.0.1"); err != nil {
		t.Fatalf("Correlate: %v", err)
	}
	if len(alerts.upserted) != 1 || alerts.upserted[0].TripCount != 1 {
		t.Fatalf("beklenen: 1 alarm, trip_count=1; alindi: %+v", alerts.upserted)
	}
}
