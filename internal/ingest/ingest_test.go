package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// fakeStore, Store sozlesmesinin test ikamesidir.
type fakeStore struct {
	inserted  []trap.TripEvent
	duplicate bool
	err       error
}

func (f *fakeStore) InsertTripEvent(_ context.Context, ev trap.TripEvent) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.duplicate {
		return false, nil
	}
	f.inserted = append(f.inserted, ev)
	return true, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func validEvent() trap.TripEvent {
	return trap.TripEvent{
		EventID:    "evt-001",
		TrapID:     "b0d3f1e2-0000-0000-0000-000000000001",
		Sensor:     "sensor-ssh",
		Source:     "10.0.0.99",
		ObservedAt: time.Now().UTC(),
	}
}

func TestHandle_ValidEventInserted(t *testing.T) {
	fs := &fakeStore{}
	c := New(fs, testLogger())

	data, _ := json.Marshal(validEvent())
	if err := c.Handle(context.Background(), data); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(fs.inserted) != 1 {
		t.Fatalf("insert sayisi = %d, istenen 1", len(fs.inserted))
	}
	if fs.inserted[0].EventID != "evt-001" {
		t.Errorf("event_id = %q, istenen evt-001", fs.inserted[0].EventID)
	}
}

func TestHandle_InvalidJSON(t *testing.T) {
	fs := &fakeStore{}
	c := New(fs, testLogger())

	if err := c.Handle(context.Background(), []byte("{bozuk")); err == nil {
		t.Fatal("bozuk JSON hata dondurmeliydi")
	}
	if len(fs.inserted) != 0 {
		t.Error("bozuk JSON store'a ulasmamali")
	}
}

func TestHandle_MissingFields(t *testing.T) {
	cases := map[string]func(*trap.TripEvent){
		"event_id":    func(ev *trap.TripEvent) { ev.EventID = "" },
		"trap_id":     func(ev *trap.TripEvent) { ev.TrapID = "" },
		"sensor":      func(ev *trap.TripEvent) { ev.Sensor = "" },
		"source":      func(ev *trap.TripEvent) { ev.Source = "" },
		"observed_at": func(ev *trap.TripEvent) { ev.ObservedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fs := &fakeStore{}
			c := New(fs, testLogger())

			ev := validEvent()
			mutate(&ev)
			data, _ := json.Marshal(ev)

			if err := c.Handle(context.Background(), data); err == nil {
				t.Errorf("%s eksikken hata bekleniyordu", name)
			}
			if len(fs.inserted) != 0 {
				t.Errorf("%s eksikken store'a yazilmamali", name)
			}
		})
	}
}

func TestHandle_DuplicateIsNotError(t *testing.T) {
	fs := &fakeStore{duplicate: true}
	c := New(fs, testLogger())

	data, _ := json.Marshal(validEvent())
	if err := c.Handle(context.Background(), data); err != nil {
		t.Fatalf("yinelenen event hata olmamali (idempotentlik): %v", err)
	}
}

func TestHandle_OnInsertedCalledForNewTrip(t *testing.T) {
	fs := &fakeStore{}
	c := New(fs, testLogger())

	var got trap.TripEvent
	calls := 0
	c.OnInserted = func(_ context.Context, ev trap.TripEvent) error {
		calls++
		got = ev
		return nil
	}

	data, _ := json.Marshal(validEvent())
	if err := c.Handle(context.Background(), data); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls != 1 {
		t.Fatalf("OnInserted cagri sayisi = %d, istenen 1", calls)
	}
	if got.EventID != "evt-001" {
		t.Errorf("kancaya gecen event_id = %q, istenen evt-001", got.EventID)
	}
}

func TestHandle_OnInsertedNotCalledForDuplicate(t *testing.T) {
	fs := &fakeStore{duplicate: true}
	c := New(fs, testLogger())

	calls := 0
	c.OnInserted = func(context.Context, trap.TripEvent) error {
		calls++
		return nil
	}

	data, _ := json.Marshal(validEvent())
	if err := c.Handle(context.Background(), data); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls != 0 {
		t.Errorf("yinelenen trip icin OnInserted cagrilmamali, cagri sayisi = %d", calls)
	}
}

func TestHandle_OnInsertedErrorDoesNotFailHandle(t *testing.T) {
	fs := &fakeStore{}
	c := New(fs, testLogger())
	c.OnInserted = func(context.Context, trap.TripEvent) error {
		return errors.New("korelasyon kirildi")
	}

	data, _ := json.Marshal(validEvent())
	if err := c.Handle(context.Background(), data); err != nil {
		t.Fatalf("OnInserted hatasi Handle'i basarisiz kilmamali: %v", err)
	}
	if len(fs.inserted) != 1 {
		t.Error("trip yine de kalici olarak yazilmis olmali")
	}
}

func TestHandle_StoreError(t *testing.T) {
	fs := &fakeStore{err: errors.New("db koptu")}
	c := New(fs, testLogger())

	data, _ := json.Marshal(validEvent())
	if err := c.Handle(context.Background(), data); err == nil {
		t.Fatal("store hatasi Handle'dan donmeliydi")
	}
}
