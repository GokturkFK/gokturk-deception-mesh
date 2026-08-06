# Otomatik Tuzak Dağıtımı — Seed Profilleri (APP-12)

EPIC H'nin güvenlik tarafı. Bu doküman **APP-12**'nin ürettiği sözleşmeyi tanımlar;
kimliği hedefe fiilen yerleştiren transport **OPS-11**'in işidir ve bu sözleşmeyi tüketir.

| Katman | Görev | Sahip | Nerede |
|---|---|---|---|
| Kimliğin **şekli** ve tespit-edilemezlik politikası | APP-12 | @fetihcakmak | `internal/seed` |
| Kimliği hedefe **yerleştiren** transport (SSH, envanter, revoke) | OPS-11 | @uzunkubra50 | `cmd/seeder`, `internal/seeder` |

---

## 1. Sorun: varsayılan canary bir hedefe ekilemez

APP-2'nin ürettiği ad `svc_` + 12 hex'tir (`svc_a3f9b2c1d4e5`). API'den provision edip
elle bir yere koyarken sorun değil; ama **bir sunucuya ekildiğinde sırıtır**:

- Gerçek sistemlerde hiçbir servis hesabı rastgele hex bloğu taşımaz.
- Tek tip `svc_` öneki **tüm tuzakları birbirine bağlar**: saldırgan birini fark ederse
  ağdaki her tuzağı tanır.

Tespit edilen bir yem, yem olmaktan çıkar. APP-12 bu yüzden vardır.

## 2. Bilinçli takas: entropi ↓, inandırıcılık ↑

Profiller kullanıcı adını **kasıtlı olarak düşük entropili** üretir (küçük bir gerçekçi
isim havuzu + gerekirse kısa sayısal son ek: `backup`, `monitor03`). Bu güvenlidir:

- **Adın tahmin edilebilirliği zafiyet değil:** hesaba girmek için yüksek entropili
  secret gerekir; secret üretimi değişmedi (24 bayt rastgele).
- **Adın herhangi bir kullanımı zaten alarmdır** (sıfır-FP tezi). Saldırganın adı
  bilmesi bize *yarar* sağlar — yem çekici olmalıdır (MITRE Engage: *Lures*).
- Buna karşılık **yüksek entropili ad hesabı ele verir** — asıl kayıp budur.

Benzersizlik entropiden değil, `Existing` kümesinden (hedefteki mevcut hesaplar) ve
DB'deki unique kısıttan gelir.

## 3. Profiller

| Profil | UID | Shell | Home | `last`'ta görünür mü |
|---|---|---|---|---|
| `debian-service` (varsayılan) | 100–999 | `/usr/sbin/nologin` | `/var/lib/<ad>` | Hayır |
| `rhel-service` | 100–999 | `/sbin/nologin` | `/var/lib/<ad>` | Hayır |
| `ops-user` | 1000–60000 | `/bin/bash` | `/home/<ad>` | **Evet — risk** |

**Neden servis profilleri varsayılan:** `last`/`lastlog` çıktısında bir nologin servis
hesabının **hiç görünmemesi normaldir**. İnteraktif bir hesabın ise login geçmişi olması
beklenir; hiç girişi olmayan bir insan hesabı `last`'ta dikkat çeker ve wtmp geçmişi
uydurmak kırılgan/izinvasif bir iştir. `ops-user` daha çekici bir yemdir ama bu artık
riski taşır — `DetectabilityIssues` bunu açıkça bildirir, sessizce gizlemez.

**Yaşlandırma:** taze oluşturulmuş bir hesap, yıllardır duran hesapların yanında zaman
damgasından ele verir. Profil bir `MinAge`/`MaxAge` aralığı tanımlar; üretilen
`Identity.SeededAt` bu aralıktan geriye tarihlenir. **OPS-11 home dizini zaman
damgalarını `SeededAt`'e göre ayarlamalıdır.**

## 4. OPS-11'in tükettiği sözleşme

```go
p, _ := seed.Get("debian-service")          // veya seed.Default()

// Hedeften okunan mevcut hesaplar — çarpışmayı (gerçek hesabın üzerine yazma) önler.
ex := seed.Existing{Usernames: ..., UIDs: ...}

id, err := p.NewIdentity(time.Now().UTC(), ex)
// id.Username / UID / GID / Shell / Home / GECOS / SeededAt

id.PasswdLine()   // "backup:x:142:142:backup service account,,,:/var/lib/backup:/usr/sbin/nologin"
```

Canary'yi üretirken profil adını provider'a bağlayın:

```go
provider := trap.NewCredentialCanaryProvider(hmacKey,
    trap.WithUsernameGenerator(func() (string, error) { return id.Username, nil }))
```

Yerleştirmeden önce **mutlaka** doğrulayın:

```go
if issues := id.DetectabilityIssues(p, time.Now()); len(issues) > 0 {
    // hesap ayırt edilebilir — seed etmeyin veya profili düzeltin
}
```

### OPS-11 implementasyonu (`cmd/seeder`, `internal/seeder`)

Yukarıdaki sözleşmeyi tüketen transport tamamlandı: bir operator CLI'ı, SSH ile hedefe
bağlanıp kimliği **gerçekten** yerleştirir/kaldırır — control-api gibi sürekli çalışan bir
servis değildir, hedef başına elle çalıştırılır.

```sh
# .env'deki ile AYNI HMAC_KEY (control-api tuzakları doğru dogrulayabilsin diye)
export DB_DSN=postgres://gokturk:gokturk@localhost:5432/gokturk?sslmode=disable
export HMAC_KEY=...

# Seed: hedefe bağlan, mevcut hesapları oku, çarpışmayan bir kimlik üret,
# DetectabilityIssues boşsa tuzağı provision edip hedefe gerçekten yerleştir.
go run ./cmd/seeder seed -target 10.0.0.50:22 -ssh-user root \
  -ssh-key ~/.ssh/id_ed25519 -profile debian-service

# Revoke: hesabı hedeften kaldırır + traps.revoked_at (sensör resolver'ı
# iptal edilmiş tuzağı zaten atlar, bkz. internal/sensorssh/resolver.go).
go run ./cmd/seeder revoke -target 10.0.0.50:22 -ssh-user root -ssh-key ~/.ssh/id_ed25519
```

Sorumluluklar ve nasıl karşılandıkları:

- **SSH transport, hedef envanteri:** `internal/seeder/ssh.go` (host key doğrulaması
  `known_hosts`'a karşı — `InsecureIgnoreHostKey` KULLANILMAZ); envanter
  `seed_targets` tablosunda (`host`/`ssh_user`/`ssh_key_ref`/`profile`,
  bkz. `migrations/00002_seed_targets.sql`).
- **İdempotency:** `host` üzerinde partial unique index (`revoked_at IS NULL`) DB
  seviyesinde garanti eder; `seeder seed` ikinci çağrıda yeni hesap açmadan mevcut
  kaydı raporlayıp çıkar.
- **Revoke:** hesabı (+ home dizini) hedeften kaldırır, `seed_targets.revoked_at` ve
  `traps.revoked_at`'i günceller. `userdel --remove` kullanıcının özel grubunu zaten
  kendisi temizler (shadow-utils standart davranışı) — ayrı bir `groupdel` YANLIŞTIR
  (gerçek bir hedefte denenip düzeltildi, bkz. `internal/seeder/target.go`).
- **SSH private key düz metin bulunmaz:** anahtar yalnızca operatörün yerel
  dosya sisteminde durur; `seed_targets.ssh_key_ref` sadece dosya YOLUNU tutar,
  anahtarın kendisini değil.
- **Home dizini zaman damgaları:** `Apply`, `SeededAt`'e göre `touch -t` ile geriye
  tarihler.

Gerçek bir SSH hedefine (Docker'da openssh-server çalıştıran bir Debian container)
karşı uçtan uca doğrulandı: seed → hesap/grup/shadow doğru oluşuyor, home dizini
geriye tarihleniyor, ikinci seed idempotent, revoke hesabı tam olarak kaldırıyor.

## 5. Sensör garantisi

Profil üretimi ne kadar "inandırıcı" olursa olsun, sensör zincirini kırarsa tuzak
körleşir. `internal/seed/sensor_compat_test.go` bu sözleşmeyi kilitler: her profilden
üretilen ad için

```
seed.NewIdentity → auth.log satırı → ParseAuthLine → Decode → TripEvent
```

zinciri doğrulanır (`Failed`, `Accepted` ve `invalid user` biçimleri dahil). Ayrıca
seed profilindeki bir ad **tuzak olarak kayıtlı değilse** hiçbir şey üretilmediği
(sıfır-FP) test edilir. Üretilen adlar POSIX `^[a-z_][a-z0-9_-]{0,31}$` biçimine
uyar — boşluk içermemesi parser'ın `\S+` yakalaması için kritiktir.

## 6. Artık riskler

- **`ops-user` profili:** boş `last` geçmişi (yukarıda).
- **Havuz tükenmesi:** hedefte tüm taban adlar doluysa sayısal son eke düşülür
  (`backup03`); bu hâlâ gerçekçidir ama havuz küçük olduğu için çok sayıda tuzak
  eken bir ortamda desen fark edilebilir hale gelebilir → havuzu büyütün.
- **Profil/dağıtım uyumsuzluğu:** RHEL hedefe `debian-service` (nologin yolu farklı)
  ekmek sırıtır; OPS-11 hedef envanterinde dağıtımı tutup doğru profili seçmelidir.
