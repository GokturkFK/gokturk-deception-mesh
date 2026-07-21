package seed

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// usernameRe, hedef sistemlerde kabul edilen (ve sensorun parse edebildigi)
// POSIX kullanici adi bicimidir. Bosluk icermemesi APP-4 parser'i icin
// kritiktir: authLineRe kullanici adini \S+ ile yakalar.
var usernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// hexBlobRe, "svc_a3f9b2c1d4e5" gibi yuksek entropili adlari yakalar —
// tuzagi ele veren en belirgin imza (bkz. paket dokumani).
var hexBlobRe = regexp.MustCompile(`[0-9a-f]{8,}`)

// Existing, hedef sunucuda halihazirda bulunan hesaplardir. OPS-11 seeding
// oncesi hedeften okuyup verir; carpismayi (gercek bir hesabin uzerine yazma)
// onler.
type Existing struct {
	Usernames map[string]bool
	UIDs      map[int]bool
}

func (e Existing) hasUser(u string) bool { return e.Usernames != nil && e.Usernames[u] }
func (e Existing) hasUID(id int) bool    { return e.UIDs != nil && e.UIDs[id] }

// Identity, hedefe ekilecek somut hesabin tam tanimidir. OPS-11 bunu tuketip
// hesabi gercekten olusturur (bkz. PasswdLine).
type Identity struct {
	Profile  string
	Username string
	UID      int
	GID      int
	Shell    string
	Home     string
	GECOS    string

	// SeededAt, hesabin "olusturulmus gibi gorunecegi" (geriye tarihli) an.
	// OPS-11 home dizini zaman damgalarini buna gore ayarlamalidir.
	SeededAt time.Time
}

// NewIdentity, profile uygun, hedefte carpismayan bir kimlik uretir.
func (p Profile) NewIdentity(now time.Time, ex Existing) (Identity, error) {
	if err := p.Validate(); err != nil {
		return Identity{}, err
	}

	username, err := p.newUsername(ex)
	if err != nil {
		return Identity{}, err
	}
	uid, err := p.newUID(ex)
	if err != nil {
		return Identity{}, err
	}
	seededAt, err := p.backdate(now)
	if err != nil {
		return Identity{}, err
	}

	base := strings.TrimRight(p.HomeBase, "/")
	id := Identity{
		Profile:  p.Name,
		Username: username,
		UID:      uid,
		GID:      uid, // hesapla ayni adli grup: Debian/RHEL'de yaygin desen
		Shell:    p.Shell,
		Home:     base + "/" + username,
		GECOS:    fmt.Sprintf(p.GECOSFormat, username),
		SeededAt: seededAt,
	}
	if err := id.Validate(); err != nil {
		return Identity{}, err
	}
	return id, nil
}

// newUsername, once sade bir taban ad dener (en inandirici hali: "backup"),
// hepsi doluysa kisa sayisal son ek ekler ("backup03"). Entropi kasitli olarak
// dusuktur; gerekce paket dokumaninda.
func (p Profile) newUsername(ex Existing) (string, error) {
	order, err := shuffled(len(p.BaseNames))
	if err != nil {
		return "", err
	}
	for _, i := range order {
		if cand := p.BaseNames[i]; !ex.hasUser(cand) {
			return cand, nil
		}
	}
	// Tum taban adlar dolu: sayisal son ek ile dene.
	for _, i := range order {
		for n := 1; n <= 99; n++ {
			cand := fmt.Sprintf("%s%02d", p.BaseNames[i], n)
			if !ex.hasUser(cand) {
				return cand, nil
			}
		}
	}
	return "", fmt.Errorf("seed: %s: bos kullanici adi bulunamadi (hedefte %d ad dolu)", p.Name, len(ex.Usernames))
}

func (p Profile) newUID(ex Existing) (int, error) {
	span := p.UIDMax - p.UIDMin + 1
	// Once rastgele dene; yogun araliklarda sirali taramaya dus.
	for range 64 {
		n, err := randIntn(span)
		if err != nil {
			return 0, err
		}
		if uid := p.UIDMin + n; !ex.hasUID(uid) {
			return uid, nil
		}
	}
	for uid := p.UIDMin; uid <= p.UIDMax; uid++ {
		if !ex.hasUID(uid) {
			return uid, nil
		}
	}
	return 0, fmt.Errorf("seed: %s: %d-%d araliginda bos UID yok", p.Name, p.UIDMin, p.UIDMax)
}

// backdate, [MinAge, MaxAge] araligindan rastgele bir yas secip hesabin
// gorunur olusturulma anini geriye tasir.
func (p Profile) backdate(now time.Time) (time.Time, error) {
	span := int(p.MaxAge-p.MinAge) / int(time.Hour)
	if span <= 0 {
		return now.Add(-p.MinAge), nil
	}
	n, err := randIntn(span)
	if err != nil {
		return time.Time{}, err
	}
	return now.Add(-p.MinAge - time.Duration(n)*time.Hour), nil
}

// Validate, kimligin hem hedef sistemde gecerli hem de sensor tarafindan
// parse edilebilir oldugunu dogrular.
func (i Identity) Validate() error {
	switch {
	case !usernameRe.MatchString(i.Username):
		return fmt.Errorf("seed: gecersiz kullanici adi %q (POSIX disi veya bosluk iceriyor — sensor parse edemez)", i.Username)
	case i.UID <= 0:
		return fmt.Errorf("seed: gecersiz UID %d", i.UID)
	case i.Shell == "" || i.Shell[0] != '/':
		return fmt.Errorf("seed: gecersiz shell %q", i.Shell)
	case i.Home == "" || i.Home[0] != '/':
		return fmt.Errorf("seed: gecersiz home %q", i.Home)
	case strings.ContainsAny(i.GECOS, ":\n"):
		return fmt.Errorf("seed: GECOS ':' veya yeni satir iceremez (passwd satirini bozar)")
	}
	return nil
}

// PasswdLine, kimligin /etc/passwd karsiligini uretir. OPS-11'in tuketecegi
// somut sozlesme budur (dogrudan yazmak yerine useradd parametrelerine de
// cevrilebilir).
func (i Identity) PasswdLine() string {
	return fmt.Sprintf("%s:x:%d:%d:%s:%s:%s", i.Username, i.UID, i.GID, i.GECOS, i.Home, i.Shell)
}

// DetectabilityIssues, kimligin gercek bir hesaptan AYIRT EDILEBILIR olmasina
// yol acan sebepleri dondurur. Bos donmesi APP-12'nin birinci kabul
// kriteridir: "/etc/passwd gibi standart araclarla ayirt edilemiyor".
func (i Identity) DetectabilityIssues(p Profile, now time.Time) []string {
	var issues []string

	if hexBlobRe.MatchString(i.Username) {
		issues = append(issues, fmt.Sprintf(
			"kullanici adi yuksek entropili blok iceriyor (%q): gercek hesaplarda gorulmez, tuzagi ele verir", i.Username))
	}
	if strings.HasPrefix(i.Username, "svc_") {
		issues = append(issues, `"svc_" on eki tum tuzaklari birbirine baglar: biri fark edilirse hepsi tanilanir`)
	}
	if i.UID < p.UIDMin || i.UID > p.UIDMax {
		issues = append(issues, fmt.Sprintf(
			"UID %d, %s profilinin %d-%d araliginin disinda: hesap sinifi passwd'de tutarsiz gorunur",
			i.UID, p.Name, p.UIDMin, p.UIDMax))
	}
	if i.Shell != p.Shell {
		issues = append(issues, fmt.Sprintf(
			"shell %q, profilin beklenen %q degeriyle uyusmuyor", i.Shell, p.Shell))
	}
	if want := strings.TrimRight(p.HomeBase, "/") + "/" + i.Username; i.Home != want {
		issues = append(issues, fmt.Sprintf("home %q beklenen %q degil", i.Home, want))
	}
	if age := now.Sub(i.SeededAt); age < p.MinAge {
		issues = append(issues, fmt.Sprintf(
			"hesap cok taze gorunuyor (%s): yillardir duran hesaplarin yaninda zaman damgasindan ele verir, en az %s geriye tarihlenmeli",
			age.Round(time.Hour), p.MinAge))
	}
	if p.AppearsInLast {
		issues = append(issues, fmt.Sprintf(
			"%s interaktif bir profil: hesabin `last`/`lastlog` gecmisi bos olacak ve bu dikkat ceker "+
				"(wtmp gecmisi uydurmak kirilgandir) — mumkunse nologin servis profili tercih edin", p.Name))
	}

	return issues
}

// randIntn, [0,n) araliginda kriptografik rastgele tamsayi dondurur.
func randIntn(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("seed: gecersiz aralik %d", n)
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, fmt.Errorf("seed: rastgele sayi uretilemedi: %w", err)
	}
	return int(v.Int64()), nil
}

// shuffled, [0,n) indekslerinin rastgele siralanmis halini dondurur.
func shuffled(n int) ([]int, error) {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j, err := randIntn(i + 1)
		if err != nil {
			return nil, err
		}
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
