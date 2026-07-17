package sensorssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// CanaryLookup, bir kullanicinin canary olup olmadigini ve trap_id'sini verir.
type CanaryLookup interface {
	TrapID(username string) (trapID string, ok bool)
}

// Decoder, SSH parola denemelerini TripEvent'e cevirir (trap.Decoder sozlesmesi).
// Yalnizca bilinen bir canary kullanicisiyla yapilan denemeler trip uretir;
// digerleri ErrNotATrip dondurur. Sifir-FP tezinin kod duzeyindeki karsiligi:
// canary olmayan hicbir giris (basarili ya da basarisiz) alarma donusemez.
type Decoder struct {
	canaries CanaryLookup
	sensor   string
}

// NewDecoder, verilen canary aramasi ve varsayilan sensor adiyla bir Decoder olusturur.
func NewDecoder(canaries CanaryLookup, sensor string) *Decoder {
	return &Decoder{canaries: canaries, sensor: sensor}
}

// Decode, ham gozlemi parse eder ve gozlem bir canary tetiklemesiyse TripEvent
// uretir; degilse trap.ErrNotATrip doner.
func (d *Decoder) Decode(obs trap.RawObservation) (*trap.TripEvent, error) {
	ev, ok := ParseAuthLine(obs.Line)
	if !ok {
		return nil, trap.ErrNotATrip
	}
	trapID, ok := d.canaries.TrapID(ev.Username)
	if !ok {
		return nil, trap.ErrNotATrip
	}

	sensor := obs.Sensor
	if sensor == "" {
		sensor = d.sensor
	}
	return &trap.TripEvent{
		EventID:    eventID(obs.Line),
		TrapID:     trapID,
		Sensor:     sensor,
		Source:     ev.Source,
		ObservedAt: obs.ObservedAt,
		Raw:        rawPayload(ev),
	}, nil
}

// eventID, ham log satirindan deterministik bir kimlik uretir. Ayni fiziksel
// satir tekrar okunursa ayni id cikar -> ingest tarafinda event_id unique
// kisiti sayesinde idempotentlik (PLAN APP-6).
func eventID(line string) string {
	sum := sha256.Sum256([]byte(line))
	return hex.EncodeToString(sum[:])
}

func rawPayload(ev AuthEvent) json.RawMessage {
	b, err := json.Marshal(struct {
		Username string `json:"username"`
		Accepted bool   `json:"accepted"`
	}{Username: ev.Username, Accepted: ev.Accepted})
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
