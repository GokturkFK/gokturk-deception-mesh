package seed_test

// APP-12'nin 2. kabul kriteri: seed edilen hesaba yapilan giris denemesi,
// APP-4/5 sensoru tarafindan HALA dogru sekilde yakalanip TripEvent'e
// cevrilmelidir. Profil uretimi ne kadar "inandirici" olursa olsun, sensor
// zincirini kirarsa tuzak korlesmis olur.
//
// Bu test seed <-> sensorssh sozlesmesini kilitler: profil uretimi degisirse
// (yeni karakterler, farkli desen) burasi kirmalidir.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/seed"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/sensorssh"
	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

type staticLookup map[string]string

func (m staticLookup) TrapID(u string) (string, bool) {
	id, ok := m[u]
	return id, ok
}

func TestSeededAccount_StillDetectedBySensor(t *testing.T) {
	const (
		trapID     = "trap-seeded-1"
		attackerIP = "198.51.100.23"
	)
	now := time.Now().UTC()

	for _, name := range seed.Names() {
		p, err := seed.Get(name)
		if err != nil {
			t.Fatalf("Get(%s): %v", name, err)
		}
		id, err := p.NewIdentity(now, seed.Existing{})
		if err != nil {
			t.Fatalf("%s: NewIdentity: %v", name, err)
		}

		decoder := sensorssh.NewDecoder(staticLookup{id.Username: trapID}, "sensor-ssh")

		// sshd, hesap VARSA "Failed password for USER", hesap yoksa
		// "Failed password for invalid user USER" yazar; ikisi de yakalanmali.
		lines := []string{
			"May 10 12:00:00 host sshd[1]: Failed password for " + id.Username + " from " + attackerIP + " port 5 ssh2",
			"May 10 12:00:01 host sshd[1]: Accepted password for " + id.Username + " from " + attackerIP + " port 6 ssh2",
			"May 10 12:00:02 host sshd[1]: Failed password for invalid user " + id.Username + " from " + attackerIP + " port 7 ssh2",
		}
		for _, line := range lines {
			ev, err := decoder.Decode(trap.RawObservation{
				Sensor: "sensor-ssh", Line: line, ObservedAt: now,
			})
			if err != nil {
				t.Fatalf("%s: seed edilen hesap (%q) sensorden gecmedi: %v\nsatir: %s",
					name, id.Username, err, line)
			}
			if ev.TrapID != trapID {
				t.Errorf("%s: trap_id = %q, istenen %q", name, ev.TrapID, trapID)
			}
			if ev.Source != attackerIP {
				t.Errorf("%s: source = %q, istenen %q", name, ev.Source, attackerIP)
			}
		}
	}
}

// TestNonSeededAccount_StaysSilent, seeding'in sifir-FP tezini bozmadigini
// dogrular: profil havuzundaki bir ad bile OLSA, o hesap tuzak olarak
// kayitli degilse hicbir sey uretilmez.
func TestNonSeededAccount_StaysSilent(t *testing.T) {
	p := seed.Default()
	now := time.Now().UTC()
	id, err := p.NewIdentity(now, seed.Existing{})
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}

	// Lookup bos: hicbir kullanici canary degil.
	decoder := sensorssh.NewDecoder(staticLookup{}, "sensor-ssh")
	line := "May 10 12:00:00 host sshd[1]: Accepted password for " + id.Username +
		" from 10.0.0.5 port 5 ssh2"

	_, err = decoder.Decode(trap.RawObservation{Sensor: "sensor-ssh", Line: line, ObservedAt: now})
	if !errors.Is(err, trap.ErrNotATrip) {
		t.Fatalf("tuzak olmayan hesap icin ErrNotATrip beklendi, alinan: %v", err)
	}
}

// TestGeneratedUsernames_ParseableByAuthParser, uretilen adlarin ham parser
// katmaninda (APP-4) da bozulmadan cikarildigini dogrular.
func TestGeneratedUsernames_ParseableByAuthParser(t *testing.T) {
	now := time.Now().UTC()
	for _, name := range seed.Names() {
		p, _ := seed.Get(name)
		for range 20 {
			id, err := p.NewIdentity(now, seed.Existing{})
			if err != nil {
				t.Fatalf("%s: NewIdentity: %v", name, err)
			}
			line := "May 10 12:00:00 h sshd[1]: Failed password for " + id.Username + " from 1.2.3.4 port 22 ssh2"
			ev, ok := sensorssh.ParseAuthLine(line)
			if !ok {
				t.Fatalf("%s: uretilen ad parse edilemedi: %q", name, id.Username)
			}
			if ev.Username != id.Username {
				t.Fatalf("%s: parse edilen ad %q, uretilen %q", name, ev.Username, id.Username)
			}
			if strings.ContainsAny(id.Username, " \t") {
				t.Fatalf("%s: uretilen ad bosluk iceriyor: %q", name, id.Username)
			}
		}
	}
}
