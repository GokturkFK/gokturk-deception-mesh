# Tehdit Modeli & Çerçeve Eşlemesi (APP-11)

Göktürk Deception Mesh MVP v0.1'in tehdit modeli. İki bölüm: (1) sistemin _tespit etmeyi
amaçladığı_ saldırgan davranışının MITRE ATT&CK / Engage eşlemesi, (2) sistemin _kendisine_
yönelik tehditlerin STRIDE analizi.

Kapsam sınırları PROJECT PLAN.md böl. 2'deki gibidir: tek tuzak türü (`credential_canary`),
tek sensör (SSH auth). Diğer tuzak/sensör türleri v0.2+.

---

## 1. Ne tespit ediyoruz — MITRE ATT&CK

Deception mantığı şudur: hiçbir meşru kullanıcının **bilmemesi gereken** bir kimlik bilgisi
(canary) ekilir. O kimliğin _herhangi bir kullanımı_ tanım gereği kötü niyetlidir — bu, sıfır
false-positive tezinin temelidir.

| ATT&CK | Taktik | Bu projede karşılığı |
|---|---|---|
| **T1078 — Valid Accounts** | Defense Evasion / Persistence / Privilege Escalation / Initial Access | Saldırgan, ele geçirdiği canary credential'ı meşru bir hesapmış gibi kullanır. Alarmların `technique` alanı `T1078` ile etiketlenir (bkz. `internal/correlate/rule.go`). |
| **T1021.004 — Remote Services: SSH** | Lateral Movement | Kullanım vektörü SSH oturum açmadır; `sensor-ssh` `auth.log`'daki "Accepted/Failed password for USER" satırlarını izler. |
| **T1110 — Brute Force** (yan kapsam) | Credential Access | Aynı kaynaktan yinelenen başarısız denemeler korelasyonda **Critical**'a yükseltilir (kampanya birleşmesi). |

Not: canary'nin **başarısız** kullanımı bile alarm üretir — kimse o kullanıcıyı denememeli;
deneme başlı başına niyet göstergesidir.

## 2. Aldatma tarafı — MITRE Engage

[MITRE Engage](https://engage.mitre.org/) düşman katılımı (adversary engagement) çerçevesi.
Bu projenin karşılık geldiği aktiviteler:

| Engage yaklaşımı | Aktivite | Karşılık |
|---|---|---|
| **Prepare** | Decoy Credentials (sahte kimlik bilgisi) | `credential_canary` provider: rastgele `svc_` kullanıcısı + secret üretir, secret'i HMAC ile saklar. |
| **Expose** | Collection / Detection | `trip.events.v1` → ingestion → korelasyon; canary teması anında toplanıp tespite dönüşür. |
| **Elicit** | Lures (yem) | Ekilen canary, saldırganı gerçek varlıklar yerine izlenen bir yeme yönlendirir. |
| **Understand** | Threat model / Analiz | Alarmlar kaynak, ilk/son görülme ve trip sayısıyla saldırgan davranışına dair sinyal verir. |

> ML/anomali modeli **bilinçli olarak yoktur**: deception'ın değeri deterministik kesinliktir;
> istatistiksel bir model sıfır-FP tezini bozardı (PLAN böl. 2 & 10).

---

## 3. Sisteme yönelik tehditler — STRIDE

Deception platformunun kendisi de bir saldırı yüzeyidir. v0.1 durumu ve planlanan sertleştirme:

| STRIDE | Tehdit | v0.1 azaltması | Planlı sertleştirme |
|---|---|---|---|
| **Spoofing** | Sahte bir "sensör" NATS'e uydurma `TripEvent` basıp yanıltıcı alarm üretir | `TripEvent` şeması doğrulanır (ingest `validate`); air-gapped ağ | Servisler arası **mTLS** + NATS auth (OPS-9) |
| **Tampering** | `trip_events`/`alerts` kayıtları veya migration'lar değiştirilir | Versiyonlu migration (goose); şema değişikliği yalnızca yeni migration ile | En az yetkili DB kullanıcısı; DB erişim denetimi |
| **Repudiation** | Bir olayın gerçekleştiği inkâr edilir / iz kaybı | Yapılandırılmış JSON log (slog); `trip_events` kalıcı denetim izi; `event_id` idempotentliği | Değişmez log sevki (OPS-8 telemetri) |
| **Information Disclosure** | Canary secret'ı sızar ve saldırgan hangi hesapların tuzak olduğunu öğrenir | Secret **düz metin saklanmaz** — yalnızca HMAC-SHA256 özeti (`secret_hash`); `SecretHash` alanı `json:"-"`; `GET /api/v1/traps` secret_hash döndürmez | HMAC anahtarı compose secret olarak (OPS-9) |
| **Denial of Service** | `trip.events.v1`'e mesaj seli ile ingestion/DB boğulur | JetStream durable consumer + idempotent yazım; poison mesaj ack'lenir (yeniden teslim döngüsü yok) | Rate limiting; kaynak kotaları |
| **Elevation of Privilege** | Yetkisiz biri provision endpoint'ini çağırıp tuzak ekler/siler | **Token gate (APP-3):** `POST /api/v1/traps` operator token'ına kilitli — yetkisiz istek 401, read-only rol 403 (bkz. `cmd/control-api/auth.go`) | Okuma uçlarının da kilitlenmesi + mTLS (OPS-9); token yerine JWT/OIDC (v0.2) |

### Bilinen açık noktalar (v0.1)

- **Okuma uçları hâlâ korumasız** (APP-3 kısmi): `POST /api/v1/traps` artık operator token'ı
  ister (yetkisiz → 401, read-only → 403). Ancak `GET /api/v1/traps` ve `GET /api/v1/alerts`
  bilinçli olarak açık bırakıldı — içerideki sensör resolver'ı ve dashboard yalnızca GET yapar,
  bunları kilitlemek Sprint 3 mTLS'ine (OPS-9) bağlı. Okumalar secret sızdırmaz (`secret_hash`
  `json:"-"`). `OPERATOR_TOKEN` boşsa auth tamamen devre dışıdır (yalnızca air-gapped/geliştirme).
- **Servisler arası düz metin** (OPS-9): NATS ve HTTP trafiği mTLS'siz. Air-gapped iç ağ için
  MVP'de kabul, prod öncesi kapatılacak.
- **Log parse kırılganlığı**: OpenSSH sürüm/dağıtım farkları satır formatını değiştirebilir;
  `ParseAuthLine` test fixture'larıyla sağlamlaştırıldı ama yeni formatlar fixture gerektirir.

---

## Referanslar

- MITRE ATT&CK: <https://attack.mitre.org/> — T1078, T1021.004, T1110
- MITRE Engage: <https://engage.mitre.org/>
- STRIDE: Microsoft tehdit modelleme metodolojisi
