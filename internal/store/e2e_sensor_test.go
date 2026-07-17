package store

// Bu test, APP-10 e2e testini (TestEndToEnd_FakeLoginTriggersSingleCriticalAlert)
// tamamlar: o test el ile uretilmis bir TripEvent'ten baslar; bu test ise HAM bir
// auth.log satirindan baslayip sensor Decode yolunu da zincire dahil eder:
//
//	raw satir -> sensorssh.Decode -> TripEvent -> ingest -> korelasyon -> alarm
//
// Ayrica sifir-FP tezini ham satir seviyesinde kanitlar: canary OLMAYAN bir login
// satiri Decode'da ErrNotATrip verir ve hicbir sey yayilmaz -> alarm olusmaz.
//
// package store icinde tutulur (APP-10 e2e'siyle ayni gerekce): setupStore/
// applySchema ayni gercek Postgres'e DDL uygular; ayni pakette sirali kosarak
// DB yarisini onler (bkz. e2e_test.go bas notu).

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/alerting"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/ingest"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/sensorssh"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

type staticCanaryLookup map[string]string

func (m staticCanaryLookup) TrapID(u string) (string, bool) {
	id, ok := m[u]
	return id, ok
}

func TestEndToEnd_RawAuthLineToAlert(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	// Gercek bir canary provision et — username'i ham satirda kullanacagiz.
	provider := trap.NewCredentialCanaryProvider([]byte("0123456789abcdef0123456789abcdef"))
	prov, _, err := provider.Provision(ctx, "e2e")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	saved, err := s.CreateTrap(ctx, *prov)
	if err != nil {
		t.Fatalf("CreateTrap: %v", err)
	}

	// Sensor tarafi: resolver yerine gercek tuzaktan kurulmus statik esleme.
	decoder := sensorssh.NewDecoder(staticCanaryLookup{saved.Username: saved.ID}, "sensor-ssh")

	// control-api kablolamasinin birebir aynisi (cmd/control-api/main.go).
	engine := alerting.New(s, s, 0, logger)
	consumer := ingest.New(s, logger)
	consumer.OnInserted = func(ctx context.Context, ev trap.TripEvent) error {
		return engine.Correlate(ctx, ev.Source)
	}

	const attackerIP = "203.0.113.77"

	// deliver, bir ham auth satirini sensor gibi Decode edip trip ise ingest'e
	// verir; canary degilse (ErrNotATrip) sessizce hicbir sey yapmaz.
	deliver := func(line string) {
		t.Helper()
		obs := trap.RawObservation{Sensor: "sensor-ssh", Line: line, ObservedAt: time.Now().UTC()}
		ev, decErr := decoder.Decode(obs)
		if decErr != nil {
			return // ErrNotATrip: sifir-FP, yayin yok
		}
		data, mErr := json.Marshal(ev)
		if mErr != nil {
			t.Fatalf("marshal: %v", mErr)
		}
		if hErr := consumer.Handle(ctx, data); hErr != nil {
			t.Fatalf("Handle: %v", hErr)
		}
	}

	canaryLine := func(hhmmss string) string {
		return "May 10 " + hhmmss + " host sshd[1]: Failed password for " +
			saved.Username + " from " + attackerIP + " port 5 ssh2"
	}

	alertCount := func() int {
		t.Helper()
		alerts, aErr := s.ListAlerts(ctx)
		if aErr != nil {
			t.Fatalf("ListAlerts: %v", aErr)
		}
		return len(alerts)
	}

	// 1) Canary OLMAYAN gecerli bir SSH login -> alarm YOK (sifir-FP kaniti).
	deliver("May 10 12:00:00 host sshd[1]: Accepted password for gercek_kullanici from 10.0.0.5 port 5 ssh2")
	if n := alertCount(); n != 0 {
		t.Fatalf("canary olmayan login sonrasi %d alarm, istenen 0 (sifir-FP)", n)
	}

	// 2) Canary ile ilk trip -> tek High.
	deliver(canaryLine("12:01:00"))
	alerts, err := s.ListAlerts(ctx)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Severity != correlate.SeverityHigh {
		t.Fatalf("ilk canary trip sonrasi beklenen tek High degil: %+v", alerts)
	}

	// 3) Ayni kaynaktan ikinci trip -> hala tek satir, Critical'a yukselir.
	deliver(canaryLine("12:02:00"))
	alerts, err = s.ListAlerts(ctx)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("ikinci trip sonrasi %d alarm, istenen 1 (kampanya birlesmesi)", len(alerts))
	}
	if alerts[0].Severity != correlate.SeverityCritical || alerts[0].TripCount != 2 {
		t.Errorf("beklenen Critical/trip_count=2, gelen %s/%d", alerts[0].Severity, alerts[0].TripCount)
	}
	if alerts[0].Source != attackerIP {
		t.Errorf("source = %q, istenen %q", alerts[0].Source, attackerIP)
	}
}
