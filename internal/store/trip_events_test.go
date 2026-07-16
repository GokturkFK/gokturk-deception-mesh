package store

import (
	"context"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// createTestTrap, FK icin gecerli bir tuzak olusturur.
func createTestTrap(t *testing.T, s *Store, username string) trap.Trap {
	t.Helper()
	saved, err := s.CreateTrap(context.Background(), trap.Trap{
		Type: trap.TypeCredentialCanary, Username: username, SecretHash: "h", CreatedBy: "test",
	})
	if err != nil {
		t.Fatalf("test tuzagi olusturulamadi: %v", err)
	}
	return saved
}

func TestStore_InsertTripEvent_Idempotent(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tp := createTestTrap(t, s, "svc_tripowner")

	ev := trap.TripEvent{
		EventID:    "evt-idem-1",
		TrapID:     tp.ID,
		Sensor:     "sensor-ssh",
		Source:     "10.0.0.7",
		ObservedAt: time.Now().UTC(),
	}

	inserted, err := s.InsertTripEvent(ctx, ev)
	if err != nil {
		t.Fatalf("ilk insert: %v", err)
	}
	if !inserted {
		t.Error("ilk insert true donmeli")
	}

	again, err := s.InsertTripEvent(ctx, ev)
	if err != nil {
		t.Fatalf("ikinci insert hata vermemeli (ON CONFLICT DO NOTHING): %v", err)
	}
	if again {
		t.Error("ayni event_id ikinci kez true donmemeli")
	}
}

func TestStore_InsertTripEvent_UnknownTrap(t *testing.T) {
	s := setupStore(t)

	ev := trap.TripEvent{
		EventID:    "evt-orphan-1",
		TrapID:     "00000000-0000-0000-0000-000000000000", // var olmayan tuzak
		Sensor:     "sensor-ssh",
		Source:     "10.0.0.8",
		ObservedAt: time.Now().UTC(),
	}
	if _, err := s.InsertTripEvent(context.Background(), ev); err == nil {
		t.Error("var olmayan trap_id FK hatasi vermeliydi")
	}
}

func TestStore_InsertTripEvent_NilRawBecomesEmptyObject(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tp := createTestTrap(t, s, "svc_rawnil")

	ev := trap.TripEvent{
		EventID:    "evt-raw-nil",
		TrapID:     tp.ID,
		Sensor:     "sensor-ssh",
		Source:     "10.0.0.9",
		ObservedAt: time.Now().UTC(),
		Raw:        nil,
	}
	if _, err := s.InsertTripEvent(ctx, ev); err != nil {
		t.Fatalf("nil Raw ile insert: %v", err)
	}

	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT raw::text FROM trip_events WHERE event_id = $1`, ev.EventID).Scan(&raw)
	if err != nil {
		t.Fatalf("raw okunamadi: %v", err)
	}
	if raw != "{}" {
		t.Errorf("raw = %q, istenen {}", raw)
	}
}
