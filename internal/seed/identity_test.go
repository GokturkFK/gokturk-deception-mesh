package seed

import (
	"strings"
	"testing"
	"time"
)

func TestProfiles_AllValid(t *testing.T) {
	for _, name := range Names() {
		p, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		if err := p.Validate(); err != nil {
			t.Errorf("%s profili gecersiz: %v", name, err)
		}
	}
}

func TestGet_Unknown(t *testing.T) {
	if _, err := Get("yok-boyle"); err == nil {
		t.Fatal("bilinmeyen profil hata dondurmeliydi")
	}
}

// TestNewIdentity_NoDetectabilityIssues, APP-12'nin 1. kabul kriteridir:
// uretilen hesap standart araclarla (passwd alanlari, zaman damgasi) gercek
// bir hesaptan ayirt edilememeli. Servis profilleri sifir bulgu vermelidir.
func TestNewIdentity_NoDetectabilityIssues(t *testing.T) {
	now := time.Now().UTC()
	for _, name := range []string{"debian-service", "rhel-service"} {
		p, err := Get(name)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		id, err := p.NewIdentity(now, Existing{})
		if err != nil {
			t.Fatalf("%s: NewIdentity: %v", name, err)
		}
		if issues := id.DetectabilityIssues(p, now); len(issues) != 0 {
			t.Errorf("%s: hesap ayirt edilebilir, bulgular: %v (passwd: %s)", name, issues, id.PasswdLine())
		}
	}
}

// TestOpsUserProfile_WarnsAboutLastHistory, interaktif profilin artik riskini
// (bos `last` gecmisi) acikca bildirdigini dogrular — sessizce gizlenmemeli.
func TestOpsUserProfile_WarnsAboutLastHistory(t *testing.T) {
	now := time.Now().UTC()
	p, err := Get("ops-user")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	id, err := p.NewIdentity(now, Existing{})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	issues := id.DetectabilityIssues(p, now)
	if len(issues) == 0 {
		t.Fatal("interaktif profil `last` gecmisi riskini bildirmeliydi")
	}
	if !strings.Contains(strings.Join(issues, " "), "last") {
		t.Errorf("bulgular `last` gecmisinden bahsetmeli: %v", issues)
	}
}

// TestDetectabilityIssues_CatchesLegacyCanaryShape, APP-2'nin varsayilan
// `svc_`+hex adinin neden seeding icin uygun olmadigini kayit altina alir.
func TestDetectabilityIssues_CatchesLegacyCanaryShape(t *testing.T) {
	p := Default()
	now := time.Now().UTC()
	id := Identity{
		Profile: p.Name, Username: "svc_a3f9b2c1d4e5",
		UID: 150, GID: 150, Shell: p.Shell,
		Home: p.HomeBase + "/svc_a3f9b2c1d4e5", GECOS: "x",
		SeededAt: now.Add(-p.MinAge - time.Hour),
	}
	issues := id.DetectabilityIssues(p, now)
	joined := strings.Join(issues, " ")
	if !strings.Contains(joined, "entropili") {
		t.Errorf("yuksek entropili ad yakalanmali: %v", issues)
	}
	if !strings.Contains(joined, "svc_") {
		t.Errorf("svc_ on eki yakalanmali: %v", issues)
	}
}

func TestDetectabilityIssues_FreshAccountFlagged(t *testing.T) {
	p := Default()
	now := time.Now().UTC()
	id, err := p.NewIdentity(now, Existing{})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	id.SeededAt = now // geriye tarihlenmemis
	if issues := id.DetectabilityIssues(p, now); len(issues) == 0 {
		t.Error("taze olusturulmus hesap bulgu vermeliydi")
	}
}

func TestNewIdentity_AvoidsExistingAccounts(t *testing.T) {
	p := Default()
	// Tum taban adlar ve bir UID araligi dolu olsun.
	taken := map[string]bool{}
	for _, b := range p.BaseNames {
		taken[b] = true
	}
	id, err := p.NewIdentity(time.Now().UTC(), Existing{Usernames: taken})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	if taken[id.Username] {
		t.Fatalf("uretilen ad %q hedefte zaten var — gercek hesabin uzerine yazilirdi", id.Username)
	}
	if !usernameRe.MatchString(id.Username) {
		t.Errorf("son ekli ad POSIX disi: %q", id.Username)
	}
}

func TestNewIdentity_RespectsUIDRange(t *testing.T) {
	p := Default()
	for range 50 {
		id, err := p.NewIdentity(time.Now().UTC(), Existing{})
		if err != nil {
			t.Fatalf("NewIdentity: %v", err)
		}
		if id.UID < p.UIDMin || id.UID > p.UIDMax {
			t.Fatalf("UID %d, %d-%d araligi disinda", id.UID, p.UIDMin, p.UIDMax)
		}
		if id.GID != id.UID {
			t.Errorf("GID (%d) UID (%d) ile ayni olmali", id.GID, id.UID)
		}
	}
}

func TestPasswdLine_Format(t *testing.T) {
	id := Identity{
		Username: "backup", UID: 142, GID: 142,
		GECOS: "backup service account,,,", Home: "/var/lib/backup",
		Shell: "/usr/sbin/nologin",
	}
	want := "backup:x:142:142:backup service account,,,:/var/lib/backup:/usr/sbin/nologin"
	if got := id.PasswdLine(); got != want {
		t.Errorf("PasswdLine() = %q, istenen %q", got, want)
	}
	if n := strings.Count(id.PasswdLine(), ":"); n != 6 {
		t.Errorf("passwd satirinda %d ':' var, istenen 6", n)
	}
}

func TestIdentity_Validate_RejectsUnparseableUsername(t *testing.T) {
	// Bosluk iceren ad sensor parser'ini (\S+) bozar — uretimde asla cikmamali.
	id := Identity{Username: "bad name", UID: 1, Shell: "/bin/sh", Home: "/home/x"}
	if err := id.Validate(); err == nil {
		t.Fatal("bosluklu kullanici adi reddedilmeliydi")
	}
}

func TestIdentity_Validate_RejectsGECOSColon(t *testing.T) {
	id := Identity{
		Username: "backup", UID: 1, Shell: "/bin/sh",
		Home: "/home/x", GECOS: "bozuk:alan",
	}
	if err := id.Validate(); err == nil {
		t.Fatal("':' iceren GECOS reddedilmeliydi (passwd satirini bozar)")
	}
}
