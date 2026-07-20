# Göktürk Deception Mesh — MVP v0.1 Görev Planı

> İki kişilik ekip için hazırlanmış, board'a doğrudan taşınabilir görev dökümü.
> **Cyber** = güvenlik/backend çekirdeği (uygulama mantığı).
> **DevOps** = platform, teslimat, ops (CI/CD, container, gözlemlenebilirlik, air-gapped paketleme).

---

## 1. Milestone hedefi (tek cümle)

Bir güvenlik ekibi bir **credential canary** eker; bir saldırgan onu kullanır; saniyeler içinde SOC panelinde ve SIEM'de **sıfır-false-positive**, yüksek-kesinlikli bir alarm belirir — hepsi **air-gapped**, `docker compose up` ile ayağa kalkan bir stack üzerinde.

Bu çalışırsa MVP biter. Çalışmayan hiçbir "ekstra özellik" bu hedefin önüne geçmez.

---

## 2. Kapsam (v0.1)

**Dahil:**
- Tek tuzak türü: `credential_canary` (honeytoken). Çekirdek arayüz + provider yazıldı.
- Tek sensör: **SSH auth** (Linux `auth.log` / journald). Gerekçe: en hızlı air-gapped kanıt, en evrensel, demo'su en net.
- Uçtan uca hat: sensör → NATS → ingestion → korelasyon → alarm persist → panel + SIEM/Telegram.
- Modüler monolit (tek Go binary: control-API + ingestion + korelasyon in-process), NATS event bus döngüde.
- docker-compose ile tam offline stack. Testler, temel gözlemlenebilirlik, README + tehdit modeli.

**Hariç (bilinçli ertelendi):**
- Diğer tuzak türleri (network/file/DNS) → v0.2.
- Active Directory sahte servis hesabı sensörü → v0.2 (CV değeri en yüksek fast-follow).
- Korelasyonun ayrı Python servisine bölünmesi → v0.3 (sınır çizili, şimdi in-process).
- ML / anomaly model → hiçbir zaman bu katmanda (sıfır-FP tezini bozar).
- Multi-tenant, Kubernetes production → v0.3+ (Helm iskeleti şimdi atılır, deploy sonra).

---

## 3. v0.1 mimarisi (compose servisleri)

| Servis | Sahip | İçerik |
|---|---|---|
| `control-api` | Cyber | Go monolit: tuzak CRUD/provision + ingestion consumer + korelasyon + alarm persist + fan-out |
| `sensor-ssh` | Cyber | Go: auth log tailer → `RawObservation` → `Decode` → NATS publish |
| `nats` | DevOps | JetStream, tek binary |
| `postgres` | DevOps | traps / trip_events / alerts tabloları |
| `dashboard` | Cyber | Mevcut Streamlit GÖKTÜRK paneli, alarm feed'i bağlı |
| `siem-sink` (demo) | DevOps | Syslog CEF alıcısı (alarmların SIEM'e aktığını göstermek için) |

---

## 4. Önce sözleşmeleri dondur (paralel çalışmanın anahtarı)

İkiniz bloklaşmadan ilerlemek için **Sprint 0'da şu kontratlar kilitlenir**, sonra herkes kendi tarafını bağımsız yazar:

- **TripEvent JSON şeması** → `internal/trap/trap.go` (yazıldı). Değişmez wire contract.
- **Alert JSON şeması** → `internal/correlate/rule.go` (yazıldı).
- **NATS subject'leri:** `trip.events.v1` (sensör publish), `alerts.v1` (opsiyonel).
- **API sözleşmesi (OpenAPI 3):** `POST /api/v1/traps`, `GET /api/v1/traps`, `GET /api/v1/alerts`, `GET /healthz`, `GET /metrics`.
- **Postgres şeması:** `traps`, `trip_events`, `alerts` (migration dosyası olarak).

Kural: bu beş kontrat yazılı ve merge edilmeden hiçbir feature dalı açılmaz.

---

## 5. Görevler — CYBER (güvenlik/backend)

> **Güncel sahiplik (ekip kararı, 2026-07-16):** Bölüm başlıkları orijinal plandaki
> alan gruplamasını gösterir, ama fiili yürütme aşağıdaki *Sahip* etiketlerine göre.
> `@fetihcakmak` yalnızca güvenlik çekirdeğini (tuzak/canary mantığı, sensör tespit
> mantığı, auth/RBAC, tehdit modeli) yürütür; geri kalan tüm backend/frontend/test
> işleri + tüm DevOps `@uzunkubra50`'de.

### EPIC A — Control API çekirdeği
- **APP-1 · Proje iskeleti & config** — `cmd/control-api`, ortam değişkeni tabanlı config (HMAC key, DB DSN, NATS URL), graceful shutdown.
  - *Sahip:* @uzunkubra50
  - *AC:* `go run ./cmd/control-api` boot oluyor, `/healthz` 200 dönüyor.
  - *Est:* 0.5g
- **APP-2 · Tuzak provisioning endpoint** — `POST /api/v1/traps` → `CredentialCanaryProvider.Provision` → artifacts (username/secret/identity) DB'ye + response'a.
  - *Sahip:* @fetihcakmak
  - *AC:* İstek atınca DB'de kayıt oluşuyor, dönen username `svc_` ile başlıyor; `GET /api/v1/traps` listeliyor.
  - *Dep:* APP-1, Postgres şeması (OPS-2)
  - *Est:* 1g
- **APP-3 · Auth & RBAC (temel)** — API'ye JWT/OIDC veya basit token gate + rol ayrımı (operator/read-only).
  - *Sahip:* @fetihcakmak
  - *AC:* Yetkisiz istek 401; read-only rol provision yapamıyor.
  - *Est:* 1g

### EPIC B — SSH sensörü
- **APP-4 · Auth log tailer** — `cmd/sensor-ssh`: `auth.log`/journald'ı takip eden, "Accepted/Failed password for USER" satırlarını parse eden okuyucu.
  - *Sahip:* @fetihcakmak
  - *AC:* Canlı log satırından `username` doğru çıkarılıyor; rotate/truncate dayanıklı.
  - *Est:* 1g
- **APP-5 · Decode + publish** — parse edilen satır → `RawObservation` → `Decode` → trip ise `trip.events.v1`'e JSON publish.
  - *Sahip:* @fetihcakmak
  - *AC:* Canary username ile login → NATS'e bir `TripEvent` düşüyor; normal username → `ErrNotATrip`, hiçbir şey yayınlanmıyor (sıfır-FP kanıtı).
  - *Dep:* APP-4, kontratlar donmuş
  - *Est:* 1g

### EPIC C — Ingestion + korelasyon + alarm
- **APP-6 · Ingestion consumer** — control-api içinde `trip.events.v1` subscriber, gelen event'i `trip_events` tablosuna yazar, korelasyon tamponuna besler.
  - *Sahip:* @uzunkubra50
  - *AC:* Publish edilen her trip DB'ye idempotent yazılıyor (event_id unique).
  - *Est:* 1g
- **APP-7 · Korelasyon entegrasyonu** — mevcut `correlate.Evaluate`'i son N dakikanın trip'leriyle çağır, üretilen alarmları `alerts` tablosuna yaz.
  - *Sahip:* @uzunkubra50 (alarm severity mantığı `@fetihcakmak` tarafından review edilmeli)
  - *AC:* Tek trip → 1 High alarm; aynı kaynaktan 2 trip → 1 Critical alarm (mevcut testler yeşil kalır).
  - *Dep:* APP-6
  - *Est:* 1g
- **APP-8 · Bildirim fan-out** — yeni alarmı Telegram + webhook + **syslog CEF** (SIEM) çıkışlarına gönder.
  - *Sahip:* @uzunkubra50
  - *AC:* Alarm oluşunca üç kanala da düşüyor; kanal hatası diğerlerini bloklamıyor.
  - *Dep:* APP-7, `siem-sink` (OPS-6)
  - *Est:* 1g

### EPIC D — Panel & doğrulama
- **APP-9 · Dashboard alarm feed'i** — Streamlit GÖKTÜRK paneli `GET /api/v1/alerts`'i çekip canlı gösterir (severity, kaynak, ATT&CK tekniği, ilk/son görülme).
  - *Sahip:* @uzunkubra50
  - *AC:* Alarm oluşunca panelde ≤5 sn içinde beliriyor.
  - *Est:* 1g
- **APP-10 · Testler & kapsam** — provider + ingestion + korelasyon için unit; uçtan uca bir integration test (sahte login → alarm).
  - *Sahip:* @uzunkubra50 (sıfır-FP integration testi `@fetihcakmak` tarafından review edilmeli)
  - *AC:* `go test ./...` yeşil, kapsam ≥ %70 çekirdek paketlerde; `-race` temiz.
  - *Est:* 1g
- **APP-11 · Tehdit modeli & framework mapping** — MITRE Engage (deception) + ATT&CK (T1078/T1021) eşlemesi, kısa STRIDE tablosu, README mimari bölümü.
  - *Sahip:* @fetihcakmak
  - *AC:* README'de mimari diyagram + tehdit modeli + demo GIF.
  - *Est:* 1g

### EPIC H — Otomatik tuzak dağıtımı (seeding)
> Köken: @fetihcakmak'ın sorusu — "SOC ekibinin bu yemleri sunuculara elle
> koyması yerine otomatik bir dağıtım (seeding) mekanizması eklensin mi?"
> (2026-07-19). Görev ikiye ayrılır: hangi kimliğin nasıl inandırıcı
> göründüğü (güvenlik tasarımı, Cyber) vs. o kimliği hedef sunucuya
> gerçekten kim/nasıl yerleştirir (transport/altyapı, DevOps). APP-12,
> APP-2'nin `Provider` sözleşmesini genişletir; OPS-11 bunu tüketen yeni
> bir bileşendir (`cmd/seeder` veya control-api'de yeni bir endpoint).
- **APP-12 · Seed profilleri & tespit-edilemezlik politikası** — hedef OS/servis tipine göre "inandırıcı" bir kimlik şekli (kullanıcı adı deseni, shell, home dizini/dosya zaman damgaları) tanımlayan profil kuralları; seed edilen canary'nin sensöre (APP-4/5) hâlâ doğru yakalandığının garantisi.
  - *Sahip:* @fetihcakmak
  - *AC:* Seed edilen hesap, `/etc/passwd`/`last` gibi standart araçlarla gerçek bir hesaptan ayırt edilemiyor; APP-4/5 decoder'ı seed edilen hesaba giriş denemesini hâlâ doğru TripEvent'e çeviriyor.
  - *Dep:* APP-2, APP-4/5
  - *Est:* 1g
- **OPS-11 · Seeding transport & envanter** — hedef sunucuya SSH ile bağlanıp canary'yi (APP-12 profiline göre) gerçekten oluşturan/kaldıran bileşen; hedef envanteri (host/ssh_user/ssh_key_ref), idempotency, revoke, SSH key'lerin güvenli saklanması.
  - *Sahip:* @uzunkubra50
  - *AC:* Bir hedef için seed tetiklenince canary hesabı o sunucuda gerçekten oluşuyor; ikinci tetikleme idempotent (yeniden oluşturmuyor); revoke ile hesap kaldırılıyor; SSH private key repo'da/imajda düz metin olarak bulunmuyor.
  - *Dep:* APP-2, APP-12
  - *Est:* 1.5g

---

## 6. Görevler — DEVOPS (platform/teslimat/ops)

> *Sahip:* Bu bölümdeki tüm görevler (OPS-1..10) @uzunkubra50.

### EPIC E — Repo & CI/CD (shift-left)
- **OPS-1 · Monorepo hijyeni** — dizin düzeni, `Makefile` (build/test/lint/run), `golangci-lint` config, `pre-commit`, conventional commits, CODEOWNERS.
  - *AC:* `make lint test` lokal tek komutla çalışıyor.
  - *Est:* 0.5g
- **OPS-2 · DB migration'ları** — `traps`/`trip_events`/`alerts` şeması, `golang-migrate` veya `goose` ile versiyonlu migration.
  - *AC:* `make migrate-up/down` çalışıyor; `trip_events.event_id` unique.
  - *Est:* 0.5g
- **OPS-3 · GitHub Actions CI** — build + `go vet` + test + `-race` + coverage gate + lint. PR'da zorunlu.
  - *AC:* Kırık test / düşük kapsam PR'ı bloklıyor.
  - *Est:* 1g
- **OPS-4 · Supply-chain guard** — container image build + **Trivy** tarama + **SBOM (syft)** + imzalama (**cosign**). (Senin DevSecOps ilgine doğrudan temas eder.)
  - *AC:* Yüksek/kritik CVE'li image publish'i bloklanıyor; SBOM artifact olarak yükleniyor.
  - *Est:* 1g

### EPIC F — Container & offline stack
- **OPS-5 · Multi-stage Dockerfile'lar** — control-api ve sensor-ssh için scratch/distroless, non-root, healthcheck.
  - *AC:* Image < ~25MB, non-root çalışıyor, `HEALTHCHECK` yeşil.
  - *Est:* 1g
- **OPS-6 · docker-compose (tam stack)** — control-api + sensor-ssh + nats + postgres + dashboard + siem-sink, healthcheck bağımlılıkları, tek `.env`.
  - *AC:* `docker compose up` → tüm servisler healthy → demo akışı uçtan uca çalışıyor.
  - *Est:* 1g
- **OPS-7 · Air-gapped bundle** — tüm image'ları `docker save` ile tarball, offline `install.sh`, offline GeoIP DB, çevrimdışı kurulum README.
  - *AC:* İnternetsiz bir makinede bundle'dan stack ayağa kalkıyor (kanıt: kurulumu ağı kapalı bir VM'de test et).
  - *Est:* 1g

### EPIC G — Gözlemlenebilirlik & güvenli iletişim
- **OPS-8 · Telemetri** — OpenTelemetry + Prometheus + Grafana; Go'da `zerolog` yapılandırılmış log; temel Grafana dashboard (trip/alarm oranı, ingest latency).
  - *AC:* `/metrics` scrape ediliyor; Grafana'da canlı alarm sayacı görünüyor.
  - *Est:* 1g
- **OPS-9 · Servisler arası mTLS (temel)** — internal CA, control-api ↔ sensor ↔ nats sertifikaları; secret/HMAC key yönetimi.
  - *AC:* Sertifikasız servis bağlanamıyor; HMAC key compose secret olarak enjekte ediliyor, repo'da düz metin yok.
  - *Est:* 1g
- **OPS-10 · K8s/Helm iskeleti (deploy v0.2)** — Helm chart taslağı (values ile image/replica/secret), sadece scaffold — "enterprise deployment" hikâyesini README'de gösterir.
  - *AC:* `helm template` geçerli manifest üretiyor (deploy şart değil).
  - *Est:* 1g

---

## 7. Sprint sıralaması

| Sprint | Süre (kaba) | Çıktı |
|---|---|---|
| **Sprint 0 — Temeller** | 3-4 gün | Kontratlar donuk, repo+CI iskeleti (OPS-1..3), compose iskeleti (OPS-6 taslak), monolit boot (APP-1), migration (OPS-2) |
| **Sprint 1 — Dikey dilim** | ~1 hafta | Sensör → NATS → ingestion → korelasyon → alarm → panelde görünür (APP-2,4,5,6,7,9). **"Çalışıyor" anı.** |
| **Sprint 2 — Production cila** | ~1 hafta | Fan-out+SIEM (APP-8), testler+kapsam (APP-10), telemetri (OPS-8), air-gapped bundle (OPS-7), supply-chain guard (OPS-4), tehdit modeli+README+demo GIF (APP-11) |
| **Sprint 3 — Enterprise hikâyesi** | ~1 hafta | mTLS (OPS-9), Helm iskeleti (OPS-10), RBAC sertleştirme (APP-3), + AD sensörü veya 2. tuzak türü (v0.2 başlangıcı) |

---

## 8. Definition of Done (MVP v0.1)

Aşağıdakilerin **hepsi** doğruysa v0.1 bitmiştir:

1. Ağı kapalı bir makinede `docker compose up` → tüm servisler healthy.
2. API'den bir credential canary provision ediliyor; dönen `svc_...` username bir hedefe ekiliyor.
3. O username ile bir SSH login simüle edildiğinde ≤5 sn içinde: panelde **High** alarm + `siem-sink`'te syslog CEF kaydı.
4. Aynı kaynaktan ikinci bir trip → panelde tek **Critical** alarm (kampanya birleşmesi).
5. Normal (canary olmayan) login → **hiçbir alarm yok** (sıfır-FP kanıtı, integration test ile de doğrulanmış).
6. `go test ./...` yeşil, `-race` temiz, çekirdek kapsam ≥ %70.
7. README: mimari diyagram + tehdit modeli (Engage/ATT&CK) + air-gapped kurulum + demo GIF.
8. CI PR'ı bloklayabiliyor; image imzalı + SBOM'lu + Trivy'den geçmiş.

---

## 9. CV / mülakat karşılığı (neden bu plan)

- **Sıfır-FP by construction** — `ErrNotATrip` ve "korelasyonda bilinçli ML yok" kararını savunabilmen, bir güvenlik mülakatında seni ayıran tekil şey.
- **Modüler monolit → mikroservis sınırı çizili** — over-engineering değil, doğru ölçekleme yargısı gösteriyor.
- **Shift-left tedarik zinciri (SBOM/Trivy/cosign)** — DevOps arkadaşının CV'sinde de somut kanıt.
- **Air-gapped bundle + mTLS** — "enterprise/on-prem gerçekten anlıyor" sinyali.
- **Engage/ATT&CK/OWASP mapping** — framework okuryazarlığı, code review'da fark edilen olgunluk.

---

## 10. Riskler & kilit noktalar

- **Log parse kırılganlığı (APP-4):** dağıtım/OpenSSH sürümüne göre satır formatı değişir → parse'ı test fixture'larıyla sağlamlaştır.
- **Kontrat kayması:** TripEvent/Alert şeması Sprint 0'dan sonra değişirse iki taraf da kırılır → şema değişikliği ancak versiyon bump (`v2`) ile.
- **Kapsam sürünmesi:** "diğer tuzak türlerini de ekleyelim" cazibesi → v0.1 DoD kilitli, yeni tür v0.2.
