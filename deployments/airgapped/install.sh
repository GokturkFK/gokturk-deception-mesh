#!/usr/bin/env sh
# install.sh — INTERNETSIZ (air-gapped) hedefte calisir (OPS-7).
#
# build-bundle.sh'in urettigi arsivden cikartilan `bundle/` dizininin
# ICINDEN calistirilir. Hicbir `docker pull`/`docker compose build`
# CAGIRMAZ — sadece onceden derlenmis imajlari (images/*.tar) yukler ve
# ayaga kaldirir. Docker Engine'in hedefte zaten kurulu oldugu varsayilir
# (bu betiğin kapsami disinda — bkz. README.md).
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE_FILE="docker-compose.yml"

echo "== [1/5] Imajlar yukleniyor (internet KULLANILMAZ) =="
for tar in images/*.tar; do
	echo "-> docker load -i $tar"
	docker load -i "$tar"
done

echo "== [2/5] .env kontrol ediliyor =="
if [ ! -f .env ]; then
	echo "-> .env yok, .env.example'dan olusturuluyor"
	cp .env.example .env
	if command -v openssl >/dev/null 2>&1; then
		KEY="$(openssl rand -hex 32)"
	else
		KEY="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
	fi
	sed "s/^HMAC_KEY=.*/HMAC_KEY=$KEY/" .env > .env.tmp && mv .env.tmp .env
	echo "-> HMAC_KEY otomatik uretildi (rastgele, 32 bayt)."
	echo "-> OPERATOR_TOKEN/READONLY_TOKEN istersen .env dosyasini duzenleyip doldur"
	echo "   (bos birakilirsa provision endpoint'i korumasiz/gelistirme modunda kalir)."
else
	echo "-> mevcut .env kullaniliyor"
fi

# POSTGRES_USER/POSTGRES_DB gibi degiskenleri bu kabuga da tasi (asagidaki
# psql cagrilari icin); docker compose ayni degiskenleri .env'den kendisi
# okur, bu sadece BU betigin kendi ihtiyaci.
set -a
# shellcheck disable=SC1091
. ./.env
set +a

echo "== [3/5] Alt yapi ayaga kaldiriliyor (nats, postgres, siem-sink) =="
docker compose -f "$COMPOSE_FILE" up -d nats postgres siem-sink

echo "-> postgres saglikli olana kadar bekleniyor"
tries=0
until docker compose -f "$COMPOSE_FILE" exec -T postgres pg_isready -U "${POSTGRES_USER:-gokturk}" >/dev/null 2>&1; do
	tries=$((tries + 1))
	if [ "$tries" -ge 30 ]; then
		echo "HATA: postgres 60 saniyede saglikli olmadi" >&2
		exit 1
	fi
	sleep 2
done

echo "== [4/5] Migration'lar uygulaniyor =="
# Betik tekrar calistirilirsa (ör. onceki bir denemeden kalan bir veri
# hacmiyle) migration'lari korkorusune yeniden uygulamaya calismak
# "relation already exists" ile patlar. goose binary'si (ve dolayisiyla
# migration takip tablosu) bu air-gapped kurulumda YOK; bunun yerine basit
# bir varlik kontrolu ile idempotent hale getiriyoruz.
SCHEMA_APPLIED="$(docker compose -f "$COMPOSE_FILE" exec -T postgres \
	psql -tAc "SELECT to_regclass('public.traps') IS NOT NULL" \
	-U "${POSTGRES_USER:-gokturk}" -d "${POSTGRES_DB:-gokturk}" | tr -d '[:space:]')"

if [ "$SCHEMA_APPLIED" = "t" ]; then
	echo "-> sema zaten uygulanmis (traps tablosu var), atlaniyor"
else
	for f in migrations/*.sql; do
		echo "-> $f"
		awk '/-- \+goose Up/{flag=1;next}/-- \+goose Down/{flag=0}flag' "$f" |
			docker compose -f "$COMPOSE_FILE" exec -T postgres \
				psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER:-gokturk}" -d "${POSTGRES_DB:-gokturk}"
	done
fi

echo "== [5/5] Uygulama servisleri ayaga kaldiriliyor (control-api, sensor-ssh, dashboard) =="
docker compose -f "$COMPOSE_FILE" up -d control-api sensor-ssh dashboard

echo
echo "Kurulum tamamlandi — internet HICBIR asamada kullanilmadi."
echo "  control-api : http://localhost:8080/healthz"
echo "  dashboard   : http://localhost:8501"
echo "  siem-sink   : docker compose -f $COMPOSE_FILE logs siem-sink"
echo
echo "Not: sensor-ssh /var/log/auth.log'u salt-okunur bagliyor (Debian/Ubuntu"
echo "formati varsayilir) — bu dosya hedefte yoksa sensor-ssh'i once o servisi"
echo "durdurup elle baslatman gerekebilir."
