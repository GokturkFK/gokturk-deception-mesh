// Package integration_test, control-api'nin gercek bilesenlerini (store,
// ingest, alerting) NATS/HTTP olmadan dogrudan birbirine baglayip
// PROJECT PLAN.md APP-10'un istedigi uctan uca senaryoyu dogrular:
// "sahte login (trip event) -> alarm". Postgres erisilemezse atlanir
// (bkz. internal/store/postgres_test.go ile ayni desen).
package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/alerting"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/ingest"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/store"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

func testDSN() string {
	if v := os.Getenv("DB_DSN"); v != "" {
		return v
	}
	return "postgres://gokturk:gokturk@localhost:5432/gokturk?sslmode=disable"
}

func setupStore(t *testing.T) *store.Store {
	t.Helper()

	db, err := sql.Open("postgres", testDSN())
	if err != nil {
		t.Skipf("postgres surucusu acilamadi, integration testi atlaniyor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Skipf("postgres erisilemez, integration testi atlaniyor: %v", err)
	}

	applySchema(t, db)
	t.Cleanup(func() { _ = db.Close() })
	return store.New(db)
}

// applySchema, migration dosyasindaki Up bolumunu temiz bir sema icin uygular
// (bkz. internal/store/postgres_test.go — ayni desen, cross-package test
// oldugu icin kucuk bir kopya olarak tutulur).
func applySchema(t *testing.T, db *sql.DB) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "00001_init.sql"))
	if err != nil {
		t.Fatalf("migration dosyasi okunamadi: %v", err)
	}
	up := extractGooseUp(string(raw))

	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `ALTER TABLE IF EXISTS trip_events DROP CONSTRAINT IF EXISTS fk_trip_events_alert`)
	for _, tbl := range []string{"trip_events", "alerts", "traps"} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE"); err != nil {
			t.Fatalf("tablo temizlenemedi (%s): %v", tbl, err)
		}
	}
	if _, err := db.ExecContext(ctx, up); err != nil {
		t.Fatalf("sema uygulanamadi: %v", err)
	}
}

func extractGooseUp(s string) string {
	const upMarker = "-- +goose Up"
	start := strings.Index(s, upMarker)
	if start < 0 {
		return s
	}
	s = s[start+len(upMarker):]
	if end := strings.Index(s, "-- +goose Down"); end >= 0 {
		s = s[:end]
	}
	return s
}

// TestEndToEnd_FakeLoginTriggersSingleCriticalAlert, APP-10'un istedigi
// uctan uca senaryoyu ve sifir-FP tezini birlikte dogrular: bir canary'e
// karsi 2 "sahte login" (trip event) -> ingestion -> korelasyon -> DB'de
// TOPLAMDA tam olarak 1 alarm (baska hicbir kaynaga sizinti/FP yok).
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
