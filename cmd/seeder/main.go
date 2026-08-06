// Command seeder, APP-12'nin urettigi "inandirici" canary kimligini bir
// hedef sunucuya SSH ile GERCEKTEN yerlestiren operator CLI'idir (OPS-11,
// bkz. docs/SEEDING.md). control-api gibi surekli calisan bir servis
// DEGILDIR: bir operator tarafindan elle, hedef basina calistirilir.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/seed"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/seeder"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/store"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// hmacKeyMinLen, cmd/control-api ile ayni kisit (bkz. o dosyadaki yorum).
const hmacKeyMinLen = 32

type config struct {
	dbDSN   string
	hmacKey string
}

// loadConfig, yalnizca DB_DSN'i zorunlu tutar. HMAC_KEY sadece "seed"
// komutunda (yeni tuzak provision ederken) kullanilir; "revoke" hicbir
// zaman ihtiyac duymaz — operatoru gereksiz yere anahtar girmeye zorlamamak
// icin dogrulamasi runSeed'e birakilir.
func loadConfig() (config, error) {
	cfg := config{
		dbDSN:   os.Getenv("DB_DSN"),
		hmacKey: os.Getenv("HMAC_KEY"),
	}
	if cfg.dbDSN == "" {
		return config{}, fmt.Errorf("eksik ortam degiskeni: DB_DSN")
	}
	return cfg, nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("config yuklenemedi", "err", err)
		os.Exit(1)
	}

	db, err := sql.Open("postgres", cfg.dbDSN)
	if err != nil {
		logger.Error("postgres surucusu acilamadi", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
	st := store.New(db)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "seed":
		err = runSeed(ctx, cfg, st, logger, os.Args[2:])
	case "revoke":
		err = runRevoke(ctx, st, logger, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		logger.Error("basarisiz", "err", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `kullanim:
  seeder seed   -target host:22 -ssh-user root -ssh-key ~/.ssh/id_ed25519 [-known-hosts ~/.ssh/known_hosts] [-profile debian-service] [-created-by operator]
  seeder revoke -target host:22 -ssh-user root -ssh-key ~/.ssh/id_ed25519 [-known-hosts ~/.ssh/known_hosts]

Ortam degiskenleri: DB_DSN, HMAC_KEY (control-api ile ayni HMAC_KEY kullanilmali).`)
}

// sshFlags, seed ve revoke'un ortak baglanti parametreleridir.
type sshFlags struct {
	target     string
	sshUser    string
	sshKey     string
	knownHosts string
}

func parseSSHFlags(fs *flag.FlagSet, args []string) (sshFlags, error) {
	homeKnownHosts := os.Getenv("HOME") + "/.ssh/known_hosts"

	var f sshFlags
	fs.StringVar(&f.target, "target", "", "hedef makine (host:port)")
	fs.StringVar(&f.sshUser, "ssh-user", "root", "SSH kullanici adi")
	fs.StringVar(&f.sshKey, "ssh-key", "", "SSH private key dosya yolu")
	fs.StringVar(&f.knownHosts, "known-hosts", homeKnownHosts, "known_hosts dosya yolu")
	if err := fs.Parse(args); err != nil {
		return sshFlags{}, err
	}

	var missing []string
	if f.target == "" {
		missing = append(missing, "-target")
	}
	if f.sshKey == "" {
		missing = append(missing, "-ssh-key")
	}
	if len(missing) > 0 {
		return sshFlags{}, fmt.Errorf("eksik zorunlu bayraklar: %v", missing)
	}
	return f, nil
}

// runSeed, seed komutunun tum bayraklarini (SSH + profile + created-by) tek
// bir FlagSet uzerinde parse edip akisi calistirir.
func runSeed(ctx context.Context, cfg config, st *store.Store, logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	homeKnownHosts := os.Getenv("HOME") + "/.ssh/known_hosts"

	target := fs.String("target", "", "hedef makine (host:port)")
	sshUser := fs.String("ssh-user", "root", "SSH kullanici adi")
	sshKey := fs.String("ssh-key", "", "SSH private key dosya yolu")
	knownHosts := fs.String("known-hosts", homeKnownHosts, "known_hosts dosya yolu")
	profileName := fs.String("profile", "", "seed profili (bos ise seed.Default())")
	createdBy := fs.String("created-by", "seeder", "traps.created_by alani")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var missing []string
	if *target == "" {
		missing = append(missing, "-target")
	}
	if *sshKey == "" {
		missing = append(missing, "-ssh-key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("eksik zorunlu bayraklar: %v", missing)
	}
	if cfg.hmacKey == "" {
		return errors.New("eksik ortam degiskeni: HMAC_KEY (control-api ile ayni deger)")
	}
	if len(cfg.hmacKey) < hmacKeyMinLen {
		return fmt.Errorf("HMAC_KEY en az %d bayt olmali (mevcut %d)", hmacKeyMinLen, len(cfg.hmacKey))
	}

	// Idempotency: host icin zaten aktif bir seed varsa yeniden olusturma
	// (OPS-11 AC). DB'deki partial unique index bunu her durumda garanti
	// eder; burada erken/temiz bir mesajla ayni seyi yapariz.
	if existing, err := st.FindActiveSeedTarget(ctx, *target); err == nil {
		logger.Info("host zaten seed edilmis, atlaniyor (idempotent)",
			"target", *target, "trap_id", existing.TrapID, "profile", existing.Profile, "seeded_at", existing.SeededAt)
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("aktif seed kontrolu basarisiz: %w", err)
	}

	profile := seed.Default()
	if *profileName != "" {
		p, err := seed.Get(*profileName)
		if err != nil {
			return err
		}
		profile = p
	}

	sshTarget, err := seeder.DialSSH(*target, *sshUser, *sshKey, *knownHosts)
	if err != nil {
		return err
	}
	defer func() { _ = sshTarget.Close() }()

	existing, err := seeder.ReadExisting(ctx, sshTarget)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	identity, err := profile.NewIdentity(now, existing)
	if err != nil {
		return fmt.Errorf("kimlik uretilemedi: %w", err)
	}
	if issues := identity.DetectabilityIssues(profile, now); len(issues) > 0 {
		return fmt.Errorf("uretilen kimlik ayirt edilebilir, seed EDILMEDI: %v", issues)
	}

	provider := trap.NewCredentialCanaryProvider([]byte(cfg.hmacKey),
		trap.WithUsernameGenerator(func() (string, error) { return identity.Username, nil }))
	t, artifacts, err := provider.Provision(ctx, *createdBy)
	if err != nil {
		return fmt.Errorf("tuzak uretilemedi: %w", err)
	}
	saved, err := st.CreateTrap(ctx, *t)
	if err != nil {
		return fmt.Errorf("tuzak kaydedilemedi: %w", err)
	}

	if err := seeder.Apply(ctx, sshTarget, identity, artifacts.Secret); err != nil {
		// Hedefte gercek bir hesap olusmadi; DB'de yarim/yanlis bir tuzak
		// birakmamak icin geri al (best-effort — revoke basarisiz olsa da
		// asil hatayi (Apply) dondururuz).
		if revokeErr := st.RevokeTrap(ctx, saved.ID); revokeErr != nil {
			logger.Error("apply basarisiz oldu VE tuzak geri alinamadi (elle temizlik gerekebilir)",
				"trap_id", saved.ID, "apply_err", err, "revoke_err", revokeErr)
		}
		return fmt.Errorf("hedefe yerlestirilemedi: %w", err)
	}

	if _, err := st.CreateSeedTarget(ctx, store.SeedTarget{
		TrapID: saved.ID, Host: *target, SSHUser: *sshUser, SSHKeyRef: *sshKey, Profile: profile.Name,
	}); err != nil {
		logger.Error("seed hedefte basarili ama envanter kaydi basarisiz (elle takip gerekebilir)",
			"trap_id", saved.ID, "target", *target, "err", err)
		return fmt.Errorf("envanter kaydedilemedi: %w", err)
	}

	logger.Info("seed tamamlandi",
		"trap_id", saved.ID, "username", identity.Username, "target", *target, "profile", profile.Name)
	return nil
}

func runRevoke(ctx context.Context, st *store.Store, logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("revoke", flag.ContinueOnError)
	f, err := parseSSHFlags(fs, args)
	if err != nil {
		return err
	}

	seedTarget, err := st.FindActiveSeedTarget(ctx, f.target)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Info("bu host icin aktif bir seed kaydi yok, yapilacak bir sey yok", "target", f.target)
			return nil
		}
		return fmt.Errorf("aktif seed aranamadi: %w", err)
	}

	tp, err := st.GetTrap(ctx, seedTarget.TrapID)
	if err != nil {
		return fmt.Errorf("tuzak bulunamadi: %w", err)
	}

	sshTarget, err := seeder.DialSSH(f.target, f.sshUser, f.sshKey, f.knownHosts)
	if err != nil {
		return err
	}
	defer func() { _ = sshTarget.Close() }()

	if err := seeder.Revoke(ctx, sshTarget, tp.Username); err != nil {
		return err
	}
	if err := st.RevokeSeedTarget(ctx, seedTarget.ID); err != nil {
		return fmt.Errorf("envanter kaydi iptal edilemedi: %w", err)
	}
	if err := st.RevokeTrap(ctx, tp.ID); err != nil {
		return fmt.Errorf("tuzak iptal edilemedi: %w", err)
	}

	logger.Info("revoke tamamlandi", "trap_id", tp.ID, "username", tp.Username, "target", f.target)
	return nil
}
