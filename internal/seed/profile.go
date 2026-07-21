// Package seed, otomatik tuzak dagitiminda (EPIC H) hedef sunucuya ekilecek
// canary hesabinin NASIL GORUNECEGINI tanimlar (APP-12).
//
// Sorumluluk ayrimi: bu paket kimligin *sekli* ve tespit-edilemezlik
// politikasidir; o kimligi hedefe SSH ile gercekten yerlestiren transport
// OPS-11'in isidir (bkz. docs/SEEDING.md).
//
// # Neden profil gerekiyor
//
// APP-2'nin varsayilan canary'si `svc_` + 12 hex uretir (ornegin
// svc_a3f9b2c1d4e5). Bu, bir hedefe ekildiginde SIRITIR: gercek sistemlerde
// hicbir servis hesabi rastgele hex blogu tasimaz, ustelik tek tip on ek
// tuzaklari birbirine baglar — saldirgan bir tanesini fark ederse hepsini
// tanir. Deception'da tespit edilen bir yem, yem olmaktan cikar.
//
// # Entropi/inandiricilik takasi (bilincli guvenlik karari)
//
// Profiller kullanici adini KASITLI OLARAK dusuk entropili uretir (kucuk bir
// gercekci isim havuzu + kisa sayisal son ek). Bu guvenli, cunku:
//
//   - Kullanici adinin tahmin edilebilir olmasi bir zafiyet degildir: hesaba
//     girmek icin yuksek entropili secret gerekir (bkz. trap.randomSecret).
//   - Kullanici adinin *herhangi bir kullanimi* zaten alarmdir (sifir-FP tezi).
//     Yani saldirganin adi bilmesi bize yarar bile saglar — yem cekicidir.
//   - Buna karsilik yuksek entropili bir ad, hesabi gercek hesaplardan ayirt
//     edilebilir kilar ki asil kayip budur.
//
// Benzersizlik entropiden degil, cagiranin verdigi `taken` kumesinden ve
// DB'deki unique kisittan gelir.
package seed

import (
	"fmt"
	"time"
)

// Profile, bir hedef OS/servis sinifi icin "inandirici hesap" kurallaridir.
type Profile struct {
	Name        string
	Description string

	// UIDMin/UIDMax, hesabin dusecegi UID araligi. Servis hesaplari dagitimin
	// sistem araliginda (Debian/RHEL: 100-999) olmalidir; bu araligin disina
	// dusen bir "servis" hesabi /etc/passwd'de sirittir.
	UIDMin, UIDMax int

	// Shell, hesabin kabugu. Servis profillerinde nologin olmasi hem gercekci
	// hem de guvenlidir (asagidaki AppearsInLast notuna bakin).
	Shell string

	// HomeBase, home dizininin kok dizini (/var/lib, /home ...).
	HomeBase string

	// GECOSFormat, /etc/passwd 5. alani icin fmt sablonu (%s = taban ad).
	GECOSFormat string

	// BaseNames, gercekci taban adlar havuzu. Kucuk tutulur (inandiricilik >
	// entropi, bkz. paket dokumani).
	BaseNames []string

	// MinAge/MaxAge, hesabin "ne kadar once olusturulmus gibi gorunecegi"
	// araligidir. Taze olusturulmus bir hesap, yillardir duran hesaplarin
	// yaninda zaman damgasindan ele verir; OPS-11 bu araliktan secilen
	// SeededAt ile home dizini zaman damgalarini geriye tarihlemelidir.
	MinAge, MaxAge time.Duration

	// AppearsInLast, bu profildeki hesaplarin `last`/`lastlog` ciktisinda
	// gorunmesinin BEKLENIP beklenmedigini soyler.
	//
	// Guvenlik notu: interaktif (insan) hesaplarinin login gecmisi olmasi
	// beklenir; hic girisi olmayan bir insan hesabi `last` ciktisinda dikkat
	// ceker ve wtmp gecmisi uydurmak kirilgan/izinvasif bir istir. nologin
	// servis hesaplarinin `last`'ta hic gorunmemesi ise NORMALDIR — bu yuzden
	// varsayilan olarak servis profilleri tercih edilmelidir.
	AppearsInLast bool
}

// builtins, yerlesik profil kayit defteri.
var builtins = map[string]Profile{
	"debian-service": {
		Name:        "debian-service",
		Description: "Debian/Ubuntu sistem servis hesabi (nologin, /var/lib altinda home)",
		UIDMin:      100, UIDMax: 999,
		Shell:       "/usr/sbin/nologin",
		HomeBase:    "/var/lib",
		GECOSFormat: "%s service account,,,",
		BaseNames: []string{
			"backup", "monitor", "metrics", "archive", "collector",
			"replica", "indexer", "scheduler", "sync", "reporter",
		},
		MinAge: 90 * 24 * time.Hour, MaxAge: 720 * 24 * time.Hour,
		AppearsInLast: false,
	},
	"rhel-service": {
		Name:        "rhel-service",
		Description: "RHEL/CentOS/Rocky sistem servis hesabi (nologin, /var/lib altinda home)",
		UIDMin:      100, UIDMax: 999,
		Shell:       "/sbin/nologin",
		HomeBase:    "/var/lib",
		GECOSFormat: "%s service account",
		BaseNames: []string{
			"backup", "monitor", "metrics", "archive", "collector",
			"replica", "indexer", "scheduler", "sync", "reporter",
		},
		MinAge: 90 * 24 * time.Hour, MaxAge: 720 * 24 * time.Hour,
		AppearsInLast: false,
	},
	"ops-user": {
		Name:        "ops-user",
		Description: "Interaktif operasyon hesabi (bash, /home altinda) — en cekici yem, en riskli profil",
		UIDMin:      1000, UIDMax: 60000,
		Shell:       "/bin/bash",
		HomeBase:    "/home",
		GECOSFormat: "%s",
		BaseNames: []string{
			"dbadmin", "opsadmin", "deploy", "jenkins", "ansible",
			"buildsvc", "release", "sysops",
		},
		MinAge: 180 * 24 * time.Hour, MaxAge: 900 * 24 * time.Hour,
		AppearsInLast: true,
	},
}

// Names, yerlesik profil adlarini alfabetik dondurur.
func Names() []string {
	out := make([]string, 0, len(builtins))
	for name := range builtins {
		out = append(out, name)
	}
	// Kucuk kume; basit ekleme siralamasi yeterli.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Get, adiyla bir yerlesik profil dondurur.
func Get(name string) (Profile, error) {
	p, ok := builtins[name]
	if !ok {
		return Profile{}, fmt.Errorf("seed: bilinmeyen profil %q (mevcut: %v)", name, Names())
	}
	return p, nil
}

// Default, seeding icin varsayilan profildir. Servis profili secilir cunku
// `last` ciktisinda hic gorunmemesi normaldir (bkz. AppearsInLast).
func Default() Profile { return builtins["debian-service"] }

// Validate, profilin kendi icinde tutarli olup olmadigini dogrular.
func (p Profile) Validate() error {
	switch {
	case p.Name == "":
		return fmt.Errorf("seed: profil adi bos")
	case len(p.BaseNames) == 0:
		return fmt.Errorf("seed: %s: BaseNames bos, kullanici adi uretilemez", p.Name)
	case p.UIDMin <= 0 || p.UIDMax < p.UIDMin:
		return fmt.Errorf("seed: %s: gecersiz UID araligi %d-%d", p.Name, p.UIDMin, p.UIDMax)
	case p.Shell == "" || p.Shell[0] != '/':
		return fmt.Errorf("seed: %s: Shell mutlak yol olmali (%q)", p.Name, p.Shell)
	case p.HomeBase == "" || p.HomeBase[0] != '/':
		return fmt.Errorf("seed: %s: HomeBase mutlak yol olmali (%q)", p.Name, p.HomeBase)
	case p.MinAge <= 0 || p.MaxAge < p.MinAge:
		return fmt.Errorf("seed: %s: gecersiz yas araligi %v-%v", p.Name, p.MinAge, p.MaxAge)
	}
	return nil
}
