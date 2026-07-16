package store

import (
	"context"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

func TestStore_ListTripEventsSince_FiltersSourceAndTime(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tp := createTestTrap(t, s, "svc_window")

	now := time.Now().UTC()
	events := []trap.TripEvent{
		{EventID: "evt-old", TrapID: tp.ID, Sensor: "sensor-ssh", Source: "10.0.0.5", ObservedAt: now.Add(-1 * time.Hour)},
		{EventID: "evt-new", TrapID: tp.ID, Sensor: "sensor-ssh", Source: "10.0.0.5", ObservedAt: now},
		{EventID: "evt-other", TrapID: tp.ID, Sensor: "sensor-ssh", Source: "10.0.0.6", ObservedAt: now},
	}
	for _, ev := range events {
		if _, err := s.InsertTripEvent(ctx, ev); err != nil {
			t.Fatalf("insert %s: %v", ev.EventID, err)
		}
	}

	got, err := s.ListTripEventsSince(ctx, "10.0.0.5", now.Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("ListTripEventsSince: %v", err)
	}
	if len(got) != 1 || got[0].EventID != "evt-new" {
		t.Fatalf("beklenen: sadece evt-new; alindi: %+v", got)
	}
}

func TestStore_UpsertAlert_FirstTripCreatesHigh(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	a := correlate.Alert{
		Severity: correlate.SeverityHigh, Technique: correlate.TechniqueValidAccounts,
		Source: "10.0.0.20", Status: correlate.StatusOpen,
		FirstSeen: time.Now().UTC(), LastSeen: time.Now().UTC(), TripCount: 1,
	}
	saved, err := s.UpsertAlert(ctx, a)
	if err != nil {
		t.Fatalf("UpsertAlert: %v", err)
	}
	if saved.ID == "" {
		t.Error("ID DB tarafindan doldurulmali")
	}
	if saved.Severity != correlate.SeverityHigh {
		t.Errorf("severity = %q, istenen High", saved.Severity)
	}
}

func TestStore_UpsertAlert_SecondTripEscalatesSameRow(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	source := "10.0.0.21"
	t0 := time.Now().UTC().Add(-2 * time.Minute)
	t1 := time.Now().UTC()

	first, err := s.UpsertAlert(ctx, correlate.Alert{
		Severity: correlate.SeverityHigh, Source: source, Status: correlate.StatusOpen,
		FirstSeen: t0, LastSeen: t0, TripCount: 1,
	})
	if err != nil {
		t.Fatalf("ilk upsert: %v", err)
	}

	second, err := s.UpsertAlert(ctx, correlate.Alert{
		Severity: correlate.SeverityCritical, Source: source, Status: correlate.StatusOpen,
		FirstSeen: t0, LastSeen: t1, TripCount: 2,
	})
	if err != nil {
		t.Fatalf("ikinci upsert: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("ikinci trip yeni satir yaratti (ID %s != %s) — kampanya birlesmesi bozuldu", second.ID, first.ID)
	}
	if second.Severity != correlate.SeverityCritical {
		t.Errorf("severity = %q, istenen Critical", second.Severity)
	}
	if second.TripCount != 2 {
		t.Errorf("trip_count = %d, istenen 2", second.TripCount)
	}

	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM alerts WHERE source = $1`, source).Scan(&count); err != nil {
		t.Fatalf("sayim: %v", err)
	}
	if count != 1 {
		t.Errorf("alerts tablosunda %d satir var, istenen 1", count)
	}
}

func TestStore_UpsertAlert_ClosedAlertDoesNotBlockNewOne(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	source := "10.0.0.22"

	closed, err := s.UpsertAlert(ctx, correlate.Alert{
		Severity: correlate.SeverityHigh, Source: source, Status: correlate.StatusOpen,
		FirstSeen: time.Now().UTC(), LastSeen: time.Now().UTC(), TripCount: 1,
	})
	if err != nil {
		t.Fatalf("ilk alarm: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET status = $1 WHERE id = $2`, correlate.StatusClosed, closed.ID); err != nil {
		t.Fatalf("kapatma: %v", err)
	}

	fresh, err := s.UpsertAlert(ctx, correlate.Alert{
		Severity: correlate.SeverityHigh, Source: source, Status: correlate.StatusOpen,
		FirstSeen: time.Now().UTC(), LastSeen: time.Now().UTC(), TripCount: 1,
	})
	if err != nil {
		t.Fatalf("yeni alarm: %v", err)
	}
	if fresh.ID == closed.ID {
		t.Error("kapali alarmin uzerine yazilmamali, yeni satir olusmali")
	}
}
