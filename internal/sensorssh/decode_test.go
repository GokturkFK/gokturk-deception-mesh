package sensorssh

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

type mapLookup map[string]string

func (m mapLookup) TrapID(u string) (string, bool) {
	id, ok := m[u]
	return id, ok
}

const acceptedCanaryLine = "May 10 12:00:00 host sshd[1]: Accepted password for svc_backup from 10.0.0.99 port 5 ssh2"

func TestDecoder_CanaryTrip(t *testing.T) {
	d := NewDecoder(mapLookup{"svc_backup": "trap-123"}, "sensor-ssh")
	obs := trap.RawObservation{
		Sensor:     "sensor-ssh",
		Line:       acceptedCanaryLine,
		ObservedAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
	}

	ev, err := d.Decode(obs)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if ev.TrapID != "trap-123" {
		t.Errorf("trap_id = %q, istenen trap-123", ev.TrapID)
	}
	if ev.Source != "10.0.0.99" {
		t.Errorf("source = %q, istenen 10.0.0.99", ev.Source)
	}
	if ev.Sensor != "sensor-ssh" {
		t.Errorf("sensor = %q", ev.Sensor)
	}
	if ev.EventID == "" {
		t.Error("event_id bos olmamali")
	}
	if !ev.ObservedAt.Equal(obs.ObservedAt) {
		t.Errorf("observed_at = %v, istenen %v", ev.ObservedAt, obs.ObservedAt)
	}

	// Raw gecerli JSON olmali ve kullaniciyi icermeli.
	var raw struct {
		Username string `json:"username"`
		Accepted bool   `json:"accepted"`
	}
	if err := json.Unmarshal(ev.Raw, &raw); err != nil {
		t.Fatalf("raw cozulemedi: %v", err)
	}
	if raw.Username != "svc_backup" || !raw.Accepted {
		t.Errorf("raw beklenenle eslesmiyor: %+v", raw)
	}

	// Idempotentlik: ayni satir -> ayni event_id.
	ev2, _ := d.Decode(obs)
	if ev2.EventID != ev.EventID {
		t.Errorf("ayni satir farkli event_id uretti: %q vs %q", ev.EventID, ev2.EventID)
	}
}

func TestDecoder_NonCanaryUser_ErrNotATrip(t *testing.T) {
	// Gecerli bir SSH satiri ama kullanici canary degil -> sifir-FP.
	d := NewDecoder(mapLookup{"svc_only": "t1"}, "sensor-ssh")
	obs := trap.RawObservation{Line: acceptedCanaryLine, ObservedAt: time.Now()}

	_, err := d.Decode(obs)
	if !errors.Is(err, trap.ErrNotATrip) {
		t.Fatalf("canary olmayan kullanici icin ErrNotATrip bekleniyordu, gelen: %v", err)
	}
}

func TestDecoder_NonAuthLine_ErrNotATrip(t *testing.T) {
	d := NewDecoder(mapLookup{"svc_backup": "t1"}, "sensor-ssh")
	obs := trap.RawObservation{Line: "bu bir auth satiri degil", ObservedAt: time.Now()}

	_, err := d.Decode(obs)
	if !errors.Is(err, trap.ErrNotATrip) {
		t.Fatalf("auth olmayan satir icin ErrNotATrip bekleniyordu, gelen: %v", err)
	}
}

func TestDecoder_FallsBackToDefaultSensor(t *testing.T) {
	d := NewDecoder(mapLookup{"svc_backup": "t1"}, "varsayilan-sensor")
	obs := trap.RawObservation{Line: acceptedCanaryLine, ObservedAt: time.Now()} // Sensor bos

	ev, err := d.Decode(obs)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if ev.Sensor != "varsayilan-sensor" {
		t.Errorf("sensor = %q, bos gozlemde varsayilan kullanilmali", ev.Sensor)
	}
}
