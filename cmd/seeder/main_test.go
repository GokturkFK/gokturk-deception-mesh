package main

import (
	"context"
	"database/sql"
	"flag"
	"io"
	"log/slog"
	"testing"

	_ "github.com/lib/pq"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/store"
)

// lazyStore, gercek bir baglanti kurmadan bir *store.Store dondurur.
// database/sql.Open TEMBELDIR (ilk sorguya kadar dial ETMEZ); runSeed'in
// DB'ye dokunmadan ONCE basarisiz olmasi gereken yollarini (bos/kisa
// HMAC_KEY) gercek bir Postgres olmadan test etmeyi mumkun kilar.
func lazyStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := sql.Open("postgres", "postgres://bogus/bogus")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store.New(db)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLoadConfig_MissingDBDSN(t *testing.T) {
	t.Setenv("DB_DSN", "")

	if _, err := loadConfig(); err == nil {
		t.Fatal("beklenen hata alinmadi: DB_DSN eksikken loadConfig basarili oldu")
	}
}

func TestLoadConfig_HMACKeyNotRequired(t *testing.T) {
	// revoke HMAC_KEY kullanmaz; loadConfig bunu zorunlu tutmamali
	// (dogrulamasi runSeed'dedir, bkz. TestRunSeed_MissingHMACKey deseni).
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("HMAC_KEY", "")

	if _, err := loadConfig(); err != nil {
		t.Fatalf("HMAC_KEY bos olsa da loadConfig basarili olmali: %v", err)
	}
}

func TestLoadConfig_AllPresent(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("HMAC_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if cfg.dbDSN != "postgres://x" || cfg.hmacKey != "0123456789abcdef0123456789abcdef" {
		t.Errorf("config degerleri env ile eslesmiyor: %+v", cfg)
	}
}

func TestRunSeed_MissingHMACKey(t *testing.T) {
	cfg := config{dbDSN: "postgres://bogus", hmacKey: ""}
	args := []string{"-target", "10.0.0.1:22", "-ssh-key", "/k"}

	err := runSeed(context.Background(), cfg, lazyStore(t), discardLogger(), args)
	if err == nil {
		t.Fatal("bos HMAC_KEY ile hata beklenirdi")
	}
}

func TestRunSeed_ShortHMACKey(t *testing.T) {
	cfg := config{dbDSN: "postgres://bogus", hmacKey: "cok-kisa"}
	args := []string{"-target", "10.0.0.1:22", "-ssh-key", "/k"}

	err := runSeed(context.Background(), cfg, lazyStore(t), discardLogger(), args)
	if err == nil {
		t.Fatal("32 bayttan kisa HMAC_KEY ile hata beklenirdi")
	}
}

func TestRunSeed_MissingSSHFlags(t *testing.T) {
	cfg := config{dbDSN: "postgres://bogus", hmacKey: "0123456789abcdef0123456789abcdef"}

	err := runSeed(context.Background(), cfg, lazyStore(t), discardLogger(), nil)
	if err == nil {
		t.Fatal("-target/-ssh-key eksikken hata beklenirdi")
	}
}

func TestParseSSHFlags_MissingRequired(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(nilWriter{})

	if _, err := parseSSHFlags(fs, []string{"-ssh-user", "deploy"}); err == nil {
		t.Fatal("-target ve -ssh-key eksikken hata beklenirdi")
	}
}

func TestParseSSHFlags_AllPresent(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	f, err := parseSSHFlags(fs, []string{
		"-target", "10.0.0.1:22",
		"-ssh-user", "deploy",
		"-ssh-key", "/keys/id_ed25519",
		"-known-hosts", "/keys/known_hosts",
	})
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if f.target != "10.0.0.1:22" || f.sshUser != "deploy" || f.sshKey != "/keys/id_ed25519" || f.knownHosts != "/keys/known_hosts" {
		t.Errorf("beklenmeyen flag degerleri: %+v", f)
	}
}

func TestParseSSHFlags_DefaultSSHUser(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	f, err := parseSSHFlags(fs, []string{"-target", "10.0.0.1:22", "-ssh-key", "/k"})
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if f.sshUser != "root" {
		t.Errorf("varsayilan ssh-user = %q, istenen root", f.sshUser)
	}
}

// nilWriter, flag.FlagSet'in test sirasinda stderr'e hata basmasini bastirir.
type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) { return len(p), nil }
