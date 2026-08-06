// Package seeder, APP-12'nin urettigi seed.Identity'yi bir hedef sunucuya
// SSH uzerinden GERCEKTEN yerlestiren transport katmanidir (OPS-11, bkz.
// docs/SEEDING.md böl. 4). Kimligin sekli/inandiricilik politikasi
// internal/seed'dedir; bu paket yalnizca "nasil gonderilir" ile ilgilenir.
package seeder

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/seed"
)

// Target, bir hedef sunucuyla konusma sozlesmesidir. Gercek implementasyon
// (SSH uzerinden) ssh.go'dadir; testler bunu sahte bir implementasyonla
// karsilar, boylece Apply/Revoke/ReadExisting mantigi gercek bir SSH
// baglantisi olmadan test edilebilir.
type Target interface {
	// Run, hedefte tek bir shell komutu calistirir ve stdout'unu doner.
	// Komut sifirdan farkli cikis koduyla bitince err doner.
	Run(ctx context.Context, cmd string) (stdout string, err error)
}

// ReadExisting, hedefteki mevcut hesaplari okuyup seed.Existing'e cevirir.
// Bu, uretilecek kimligin gercek bir hesapla CARPISMAMASINI garanti eder
// (seed.Profile.NewIdentity'nin girdisi).
func ReadExisting(ctx context.Context, t Target) (seed.Existing, error) {
	out, err := t.Run(ctx, readPasswdCmd)
	if err != nil {
		return seed.Existing{}, fmt.Errorf("seeder: /etc/passwd okunamadi: %w", err)
	}

	ex := seed.Existing{Usernames: map[string]bool{}, UIDs: map[int]bool{}}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		ex.Usernames[fields[0]] = true
		if uid, err := strconv.Atoi(fields[2]); err == nil {
			ex.UIDs[uid] = true
		}
	}
	return ex, nil
}

// Apply, kimligi hedefte gercek bir Unix hesabina donusturur: once grubu,
// sonra hesabi olusturur, parolasini secret'e ayarlar, home dizinini
// Identity.SeededAt'e geriye tarihler. Adimlar sirali calisir ve ilk hatada
// durur — yarim kalan bir hesap (parola ayarlanmamis) sessizce birakilmaz,
// hata acikca doner.
func Apply(ctx context.Context, t Target, id seed.Identity, secret string) error {
	if _, err := t.Run(ctx, groupaddCmd(id)); err != nil {
		return fmt.Errorf("seeder: grup olusturulamadi (%s): %w", id.Username, err)
	}
	if _, err := t.Run(ctx, useraddCmd(id)); err != nil {
		return fmt.Errorf("seeder: hesap olusturulamadi (%s): %w", id.Username, err)
	}
	if _, err := t.Run(ctx, chpasswdCmd(id.Username, secret)); err != nil {
		return fmt.Errorf("seeder: parola ayarlanamadi (%s): %w", id.Username, err)
	}
	if _, err := t.Run(ctx, backdateCmd(id)); err != nil {
		return fmt.Errorf("seeder: home dizini geriye tarihlenemedi (%s): %w", id.Username, err)
	}
	return nil
}

// Revoke, hesabi ve home dizinini hedeften kaldirir. Grubu AYRICA silmeye
// calismaz: `userdel --remove`, kullanicinin birincil grubu adiyla ayni ve
// baska hic kimsenin uyesi degilse bu grubu ZATEN kendisi kaldirir (shadow-
// utils standart davranisi, Debian/RHEL ailesinde ortak) — ayrica bir
// groupdel cagirmak, userdel'in az once sildigi grubu tekrar silmeye
// calisip "group does not exist" hatasiyla basarisiz olur (gercek bir
// hedefe karsi calistirilarak dogrulandi).
func Revoke(ctx context.Context, t Target, username string) error {
	if _, err := t.Run(ctx, userdelCmd(username)); err != nil {
		return fmt.Errorf("seeder: hesap kaldirilamadi (%s): %w", username, err)
	}
	return nil
}
