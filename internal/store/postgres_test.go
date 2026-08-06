package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// testDSN, once DB_DSN ortam degiskenine, yoksa CI/Makefile ile ayni varsayilan
// yerel DSN'e duser. Postgres erisilemezse test atlanir (lokal, DB'siz gelistirme).
func testDSN() string {
	if v := os.Getenv("DB_DSN"); v != "" {
		return v
	}
	return "postgres://gokturk:gokturk@localhost:5432/gokturk?sslmode=disable"
}

func setupStore(t *testing.T) *Store {
	t.Helper()

	db, err := sql.Open("postgres", testDSN())
	if err != nil {
		t.Skipf("postgres surucusu acilamadi, store testi atlaniyor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Skipf("postgres erisilemez, store testi atlaniyor: %v", err)
	}

	applySchema(t, db)
	t.Cleanup(func() { _ = db.Close() })
	return New(db)
}

// applySchema, migrations/ altindaki TUM dosyalarin Up bolumunu (dosya adina
// gore sirali) temiz bir sema icin uygular. Bu ayni zamanda struct
// alanlarinin gercek sema ile uyumunu dogrular (OPS-2).
func applySchema(t *testing.T, db *sql.DB) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("migration dosyalari bulunamadi: %v", err)
	}
	sort.Strings(files)

	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `ALTER TABLE IF EXISTS trip_events DROP CONSTRAINT IF EXISTS fk_trip_events_alert`)
	for _, tbl := range []string{"seed_targets", "trip_events", "alerts", "traps"} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE"); err != nil {
			t.Fatalf("tablo temizlenemedi (%s): %v", tbl, err)
		}
	}

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("migration dosyasi okunamadi (%s): %v", f, err)
		}
		if _, err := db.ExecContext(ctx, extractGooseUp(string(raw))); err != nil {
			t.Fatalf("sema uygulanamadi (%s): %v", f, err)
		}
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

func TestStore_CreateAndList(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	in := trap.Trap{
		Type:       trap.TypeCredentialCanary,
		Username:   "svc_integration01",
		SecretHash: "deadbeefcafe",
		CreatedBy:  "tester",
	}

	saved, err := s.CreateTrap(ctx, in)
	if err != nil {
		t.Fatalf("CreateTrap: %v", err)
	}
	if saved.ID == "" {
		t.Error("ID DB tarafindan doldurulmali (uuid)")
	}
	if saved.CreatedAt.IsZero() {
		t.Error("CreatedAt DB tarafindan doldurulmali")
	}
	if saved.Username != in.Username {
		t.Errorf("username = %q, istenen %q", saved.Username, in.Username)
	}

	list, err := s.ListTraps(ctx)
	if err != nil {
		t.Fatalf("ListTraps: %v", err)
	}
	var found *trap.Trap
	for i := range list {
		if list[i].Username == in.Username {
			found = &list[i]
		}
	}
	if found == nil {
		t.Fatal("olusturulan tuzak listede bulunamadi")
	}
	if found.CreatedBy != "tester" {
		t.Errorf("createdBy = %q, istenen tester", found.CreatedBy)
	}
	// ListTraps secret_hash secmez — bellekteki alan bos olmali.
	if found.SecretHash != "" {
		t.Errorf("ListTraps secret_hash dondurmemeli, geldi: %q", found.SecretHash)
	}
}

func TestStore_GetTrap_Found(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	created := createTestTrap(t, s, "svc_gettrap01")

	got, err := s.GetTrap(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTrap: %v", err)
	}
	if got.Username != "svc_gettrap01" {
		t.Errorf("username = %q, istenen svc_gettrap01", got.Username)
	}
	if got.SecretHash != "" {
		t.Error("GetTrap secret_hash dondurmemeli")
	}
}

func TestStore_GetTrap_UnknownIDFails(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	if _, err := s.GetTrap(ctx, "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("bilinmeyen id icin hata beklenirdi")
	}
}

func TestStore_UniqueUsername(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	in := trap.Trap{Type: trap.TypeCredentialCanary, Username: "svc_dup", SecretHash: "h", CreatedBy: "x"}
	if _, err := s.CreateTrap(ctx, in); err != nil {
		t.Fatalf("ilk insert basarisiz: %v", err)
	}
	if _, err := s.CreateTrap(ctx, in); err == nil {
		t.Error("ayni username ile ikinci insert unique kisiti nedeniyle hata vermeliydi")
	}
}
