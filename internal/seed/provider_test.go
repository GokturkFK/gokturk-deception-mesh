package seed_test

// Seeding yolunun uctan uca birlesimi: profil -> trap.WithUsernameGenerator
// -> Provision. Uretilen tuzagin kullanici adi profile uymali, secret yine
// yuksek entropili olmali ve DB'ye yalnizca hash gitmelidir.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/seed"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

func TestProvider_WithSeedProfile_UsesBelievableUsername(t *testing.T) {
	p := seed.Default()
	id, err := p.NewIdentity(time.Now().UTC(), seed.Existing{})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}

	provider := trap.NewCredentialCanaryProvider(
		[]byte("0123456789abcdef0123456789abcdef"),
		trap.WithUsernameGenerator(func() (string, error) { return id.Username, nil }),
	)

	tp, artifacts, err := provider.Provision(context.Background(), "seeder")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if tp.Username != id.Username {
		t.Errorf("username = %q, profilden gelen %q olmaliydi", tp.Username, id.Username)
	}
	if strings.HasPrefix(tp.Username, "svc_") {
		t.Errorf("seeding yolunda `svc_` on eki kullanilmamali: %q", tp.Username)
	}
	// Ad dusuk entropili olsa da secret yuksek entropili kalmali —
	// guvenlik buradan gelir (bkz. seed paket dokumani).
	if len(artifacts.Secret) < 24 {
		t.Errorf("secret cok kisa (%d): ad entropisi dustu diye secret zayiflamamali", len(artifacts.Secret))
	}
	if tp.SecretHash == "" || tp.SecretHash == artifacts.Secret {
		t.Error("kalici kayit secret'i hash'li tutmali")
	}
}

// TestProvider_DefaultUnchanged, APP-12'nin APP-2 davranisini bozmadigini
// kilitler: opsiyon verilmezse eski `svc_` yolu aynen calisir.
func TestProvider_DefaultUnchanged(t *testing.T) {
	provider := trap.NewCredentialCanaryProvider([]byte("0123456789abcdef0123456789abcdef"))
	tp, _, err := provider.Provision(context.Background(), "api")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if !strings.HasPrefix(tp.Username, "svc_") {
		t.Errorf("varsayilan yol `svc_` uretmeli (APP-2 AC), alinan: %q", tp.Username)
	}
}
