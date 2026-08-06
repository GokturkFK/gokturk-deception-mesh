package seeder

import (
	"fmt"
	"strings"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/seed"
)

// shellQuote, verilen string'i POSIX sh icin tek-tirnakla guvenli sekilde
// kacisler. Uzaktaki (SSH) komut satirlarina giren HER deger buradan gecer —
// Identity alanlari zaten seed.Identity.Validate() ile kisitlanmis olsa da
// (username POSIX deseniyle, GECOS ':'/yeni satir icermez) burada AYRICA
// kacislemek savunma katmanidir: tek bir dogrulama noktasina guvenmemek icin.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// groupaddCmd, kimligin GID'iyle, kullanici adiyla ayni adli bir grup
// olusturur. useradd'in --gid secenegi (isim veya numara farketmeksizin)
// ONCEDEN VAR OLAN bir grup bekler — otomatik olusturmaz; bu yuzden
// useraddCmd'den ONCE calistirilmalidir (bkz. Apply).
func groupaddCmd(id seed.Identity) string {
	return fmt.Sprintf("groupadd --gid %s %s", shellQuote(fmt.Sprint(id.GID)), shellQuote(id.Username))
}

// useraddCmd, kimligi hedefte gercek bir Unix hesabina donusturen komuttur.
// --create-home ile home dizini de olusturulur (backdateCmd sonradan zaman
// damgalarini ayarlar). Grubun onceden var oldugu varsayilir (groupaddCmd).
func useraddCmd(id seed.Identity) string {
	return fmt.Sprintf(
		"useradd --uid %s --gid %s --home-dir %s --create-home --shell %s --comment %s %s",
		shellQuote(fmt.Sprint(id.UID)), shellQuote(fmt.Sprint(id.GID)),
		shellQuote(id.Home), shellQuote(id.Shell), shellQuote(id.GECOS), shellQuote(id.Username),
	)
}

// chpasswdCmd, hesabin parolasini duz metin secret'e ayarlar. secret
// base64.RawURLEncoding ile uretildigi icin (bkz. trap.randomSecret) zaten
// shell-guvenli bir karakter kumesindedir ([A-Za-z0-9_-]); yine de
// shellQuote uygulanir.
func chpasswdCmd(username, secret string) string {
	return fmt.Sprintf("printf '%%s:%%s' %s %s | chpasswd", shellQuote(username), shellQuote(secret))
}

// backdateCmd, home dizininin mtime/atime'ini Identity.SeededAt'e geriye
// tarihler — taze olusturulmus bir hesabin zaman damgasindan ele
// verilmemesi icin (bkz. internal/seed paket dokumani, "Yaslandirma").
func backdateCmd(id seed.Identity) string {
	ts := id.SeededAt.UTC().Format("200601021504.05")
	return fmt.Sprintf("touch -t %s %s", ts, shellQuote(id.Home))
}

// userdelCmd, hesabi ve home dizinini hedeften kaldirir. shadow-utils
// (Debian/RHEL ailesi ortak), kullanicinin birincil grubu adiyla ayniysa ve
// baska uyesi yoksa bu grubu da kendiliginden kaldirir — ayrica bir
// groupdel GEREKMEZ (bkz. Revoke dokumani).
func userdelCmd(username string) string {
	return fmt.Sprintf("userdel --remove %s", shellQuote(username))
}

// readPasswdCmd, mevcut hesap listesini okumak icin kullanilir
// (ReadExisting'in girdisi).
// nolint:gosec // G101 yanlis pozitifi: bu bir shell komutu, kimlik bilgisi degil.
const readPasswdCmd = "cat /etc/passwd"
