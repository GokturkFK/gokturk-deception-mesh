# Air-gapped kurulum (OPS-7)

Göktürk Deception Mesh'i internete hiç çıkmadan, kapalı bir ağdaki bir makinede
ayağa kaldırmak için. İki taraf var: **derleme** (internetli) ve **kurulum**
(internetsiz, hedef).

## 1. Derleme makinesinde (internet var)

Repo kökünden:

```sh
deployments/airgapped/build-bundle.sh
```

Bu betik:

1. `control-api`, `dashboard`, `sensor-ssh`, `siem-sink` imajlarını derler.
2. `nats`, `postgres` taban imajlarını çeker.
3. `docker compose config --images` ile gerçekte gereken imaj listesini alır
   (elle bakım gerekmez — `docker-compose.yml` değişse de otomatik uyum sağlar)
   ve her birini `docker save` ile `bundle/images/*.tar`'a yazar.
4. `docker-compose.yml`, `.env.example`, `migrations/`, `install.sh` ve bu
   README'i `bundle/` içine kopyalar.
5. Hepsini **tek bir dosyaya** sıkıştırır: `gokturk-airgapped-bundle.tar.gz`.

Çıktıyı (tek dosya) USB/DVD/ne varsa ile hedef ağa taşı.

## 2. Hedef makinede (internet YOK)

Ön koşul: Docker Engine + Compose plugin kurulu olmalı (bu betiğin kapsamı
dışında — işletim sistemine göre değişir, kendi offline paket kaynağınızdan
kurun).

```sh
tar xzf gokturk-airgapped-bundle.tar.gz
cd bundle
./install.sh
```

`install.sh`:

1. `images/*.tar` içindeki tüm imajları `docker load` ile yükler —
   **hiçbir `docker pull`/`docker compose build` çağırmaz.**
2. `.env` yoksa `.env.example`'dan oluşturur ve rastgele bir `HMAC_KEY` üretir
   (`openssl` yoksa `/dev/urandom`'a düşer). `OPERATOR_TOKEN`/`READONLY_TOKEN`
   isterseniz `.env`'i elle düzenleyip doldurun (boş = korumasız/geliştirme modu).
3. Alt yapıyı (`nats`, `postgres`, `siem-sink`) ayağa kaldırır, Postgres
   sağlıklı olana kadar bekler.
4. `migrations/*.sql` dosyalarındaki `-- +goose Up` bölümlerini sırayla uygular.
5. Uygulama servislerini (`control-api`, `sensor-ssh`, `dashboard`) başlatır.

Bitince:

- Panel: http://localhost:8501
- API: http://localhost:8080/healthz
- SIEM: `docker compose -f docker-compose.yml logs siem-sink`

`sensor-ssh`, hedefte `/var/log/auth.log`'u salt-okunur bağlar (Debian/Ubuntu
formatı varsayılır). Bu dosya hedefte yoksa/farklı bir yoldaysa
`docker-compose.yml`'deki `sensor-ssh.volumes`'i ona göre düzenleyin.

## Neden bu tasarım

- **Tek doğruluk kaynağı:** `build-bundle.sh` hangi imajların gerektiğini
  `docker compose config --images`'tan öğrenir; `docker-compose.yml`'e yeni
  bir servis eklendiğinde bu betikte elle güncelleme gerekmez.
- **siem-sink artık build-time:** Eski tanım `command:` içinde çalışma
  zamanında `apk add socat` yapıyordu — air-gapped bir hedefte bu, Alpine
  paket aynasına erişim gerektirdiği için servisin hiç başlamamasına yol
  açardı. `siem-sink.Dockerfile` socat'i imaja **derleme sırasında** (online
  makinede) gömer.
- **`install.sh` asla `pull`/`build` çağırmaz:** yalnızca `docker load` +
  `docker compose up` (imajlar zaten yerelde olduğu için compose onları
  yeniden derlemez/çekmez). CI'da bu, gerçekten test edilir: bundle
  oluşturulduktan sonra yerel imajlar silinip kurulumun SADECE bundle'dan
  ayağa kalktığı doğrulanır (bkz. `.github/workflows/ci.yml`, `airgapped-bundle` job'u).
