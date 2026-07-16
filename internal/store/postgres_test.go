package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, testDSN())
	if err != nil {
		t.Skipf("postgres havuzu olusturulamadi, store testi atlaniyor: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres erisilemez, store testi atlaniyor: %v", err)
	}

	applySchema(t, pool)
	t.Cleanup(pool.Close)
	return New(pool)
}

// applySchema, migration dosyasindaki Up bolumunu temiz bir sema icin uygular.
// Bu ayni zamanda struct alanlarinin gercek sema ile uyumunu dogrular (OPS-2).
func applySchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "00001_init.sql"))
	if err != nil {
		t.Fatalf("migration dosyasi okunamadi: %v", err)
	}
	up := extractGooseUp(string(raw))

	ctx := context.Background()
	_, _ = pool.Exec(ctx, `ALTER TABLE IF EXISTS trip_events DROP CONSTRAINT IF EXISTS fk_trip_events_alert`)
	for _, tbl := range []string{"trip_events", "alerts", "traps"} {
		if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE"); err != nil {
			t.Fatalf("tablo temizlenemedi (%s): %v", tbl, err)
		}
	}
	if _, err := pool.Exec(ctx, up); err != nil {
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
