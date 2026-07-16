package trap

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// Wire contract sabitleme: alan adlari degisirse bu test kirilir —
// degisiklik ancak version bump ile yapilabilir (PLAN bol. 10).
func TestTripEventWireFieldNames(t *testing.T) {
	ev := TripEvent{
		EventID:    "evt-1",
		TrapID:     "3f0e8f5e-0000-0000-0000-000000000000",
		Sensor:     "sensor-ssh",
		Source:     "10.0.0.99",
		ObservedAt: time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC),
		Raw:        json.RawMessage(`{"line":"Accepted password for svc_backup"}`),
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal hatasi: %v", err)
	}

	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("wire unmarshal hatasi: %v", err)
	}

	for _, key := range []string{"event_id", "trap_id", "sensor", "source", "observed_at", "raw"} {
		if _, ok := wire[key]; !ok {
			t.Errorf("wire contract alani eksik: %q (mevcut: %v)", key, wireKeys(wire))
		}
	}
}

func TestTripEventJSONRoundTrip(t *testing.T) {
	in := TripEvent{
		EventID:    "evt-2",
		TrapID:     "trap-2",
		Sensor:     "sensor-ssh",
		Source:     "192.0.2.7",
		ObservedAt: time.Date(2026, 7, 16, 9, 30, 0, 0, time.UTC),
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal hatasi: %v", err)
	}

	var out TripEvent
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal hatasi: %v", err)
	}

	if out.EventID != in.EventID || out.TrapID != in.TrapID ||
		out.Sensor != in.Sensor || out.Source != in.Source ||
		!out.ObservedAt.Equal(in.ObservedAt) {
		t.Errorf("round-trip esitsiz: girdi %+v, cikti %+v", in, out)
	}
}

// SecretHash hicbir kosulda API cevabina sizamaz.
func TestTrapSecretHashNeverMarshalled(t *testing.T) {
	tr := Trap{ID: "t-1", Type: TypeCredentialCanary, Username: "svc_backup", SecretHash: "cok-gizli-hash"}

	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal hatasi: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("wire unmarshal hatasi: %v", err)
	}
	if _, leaked := wire["secret_hash"]; leaked {
		t.Error("SecretHash JSON ciktisina sizdi — wire'a asla cikmamali")
	}
}

func TestErrNotATripIdentity(t *testing.T) {
	wrapped := fmt.Errorf("decode: %w", ErrNotATrip)
	if !errors.Is(wrapped, ErrNotATrip) {
		t.Error("sarilmis hata errors.Is ile ErrNotATrip olarak taninmiyor")
	}
}

func wireKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
