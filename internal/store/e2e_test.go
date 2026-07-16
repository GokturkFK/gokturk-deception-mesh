package store

// APP-10 uctan uca testi bilincli olarak bu pakette (package store) tutulur,
// ayri bir pakette DEGIL: ayri bir pakette olsaydi `go test ./...` internal/store
// ile ayni anda calisir ve ikisi de setupStore/applySchema ile ayni gercek
// Postgres'e karsi DROP+CREATE yapardi — bu, "CREATE EXTENSION" gibi DDL'lerde
// yarisa (ve CI'da goruldugu gibi "duplicate key value violates unique
// constraint pg_extension_name_index" hatasina) yol acar. Ayni pakette
// olunca test binary'si tek surecte sirali calisir, yaris ortadan kalkar.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/alerting"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/ingest"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// TestEndToEnd_FakeLoginTriggersSingleCriticalAlert, APP-10'un istedigi
// uctan uca senaryoyu ve sifir-FP tezini birlikte dogrular: bir canary'e
// karsi 2 "sahte login" (trip event) -> ingestion -> korelasyon -> DB'de
// TOPLAMDA tam olarak 1 Critical alarm (baska hicbir kaynaga sizinti/FP yok).
//
// NOT (PLAN APP-10): bu test @fetihcakmak tarafindan sifir-FP acisindan
// review edilmelidir.
func TestEndToEnd_FakeLoginTriggersSingleCriticalAlert(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	provider := trap.NewCredentialCanaryProvider([]byte("0123456789abcdef0123456789abcdef"))
	provisioned, _, err := provider.Provision(ctx, "e2e-test")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	savedTrap, err := s.CreateTrap(ctx, *provisioned)
	if err != nil {
		t.Fatalf("CreateTrap: %v", err)
	}

	// control-api'nin gercek kablolamasiyla birebir ayni (bkz.
	// cmd/control-api/main.go): ingest -> OnInserted -> alerting.Correlate.
	engine := alerting.New(s, s, 0, logger)
	consumer := ingest.New(s, logger)
	consumer.OnInserted = func(ctx context.Context, ev trap.TripEvent) error {
		return engine.Correlate(ctx, ev.Source)
	}

	const attackerIP = "203.0.113.55"
	for i := range 2 {
		ev := trap.TripEvent{
			EventID:    fmt.Sprintf("e2e-evt-%d", i),
			TrapID:     savedTrap.ID,
			Sensor:     "sensor-ssh",
			Source:     attackerIP,
			ObservedAt: time.Now().UTC(),
		}
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := consumer.Handle(ctx, data); err != nil {
			t.Fatalf("Handle (sahte login %d): %v", i, err)
		}
	}

	alerts, err := s.ListAlerts(ctx)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("DB'de %d alarm var, istenen 1 (sifir-FP: baska sizinti olmamali): %+v", len(alerts), alerts)
	}

	got := alerts[0]
	if got.Source != attackerIP {
		t.Errorf("source = %q, istenen %q", got.Source, attackerIP)
	}
	if got.Severity != correlate.SeverityCritical {
		t.Errorf("severity = %q, istenen Critical (2. trip sonrasi kampanya birlesmesi)", got.Severity)
	}
	if got.TripCount != 2 {
		t.Errorf("trip_count = %d, istenen 2", got.TripCount)
	}
	if got.Technique != correlate.TechniqueValidAccounts {
		t.Errorf("technique = %q, istenen %q", got.Technique, correlate.TechniqueValidAccounts)
	}
}
