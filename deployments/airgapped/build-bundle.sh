#!/usr/bin/env sh
# build-bundle.sh — INTERNET GEREKTIREN makinede calisir (OPS-7).
#
# Tum servis image'larini derler/ceker, docker save ile tarball'lar ve
# air-gapped bir hedefte kurulum icin gereken her seyi (compose dosyasi,
# migration'lar, install.sh, README) tek bir arsive paketler.
#
# Kullanim: deployments/airgapped/build-bundle.sh
# Cikti:    gokturk-airgapped-bundle.tar.gz (repo kokunde)
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="deployments/docker/docker-compose.yml"
BUNDLE_DIR="$REPO_ROOT/bundle"
IMAGES_DIR="$BUNDLE_DIR/images"

cd "$REPO_ROOT"

# docker compose config/build, docker-compose.yml'deki ${HMAC_KEY:?...} gibi
# zorunlu degiskenleri cozmek icin bir deger ister; bu SADECE build/derleme
# sirasinda kullanilir, kalici degildir. Gercek HMAC_KEY hedefte install.sh
# tarafindan uretilir/kullanicidan alinir.
export HMAC_KEY="${HMAC_KEY:-build-time-placeholder-0000000000000000}"

echo "== [1/5] Servis imajlari derleniyor (control-api, dashboard, sensor-ssh, siem-sink) =="
docker compose -f "$COMPOSE_FILE" build

echo "== [2/5] Taban imajlar cekiliyor (nats, postgres) =="
docker compose -f "$COMPOSE_FILE" pull nats postgres

rm -rf "$BUNDLE_DIR"
mkdir -p "$IMAGES_DIR"

echo "== [3/5] Gerekli imaj listesi cikartiliyor =="
IMAGES="$(docker compose -f "$COMPOSE_FILE" config --images)"
echo "$IMAGES"

for img in $IMAGES; do
	safe="$(echo "$img" | tr '/:' '__')"
	echo "-> docker save $img -> images/$safe.tar"
	docker save "$img" -o "$IMAGES_DIR/$safe.tar"
done

echo "== [4/5] Compose dosyasi, migration'lar ve kurulum betikleri kopyalaniyor =="
cp "$COMPOSE_FILE" "$BUNDLE_DIR/docker-compose.yml"
cp deployments/docker/.env.example "$BUNDLE_DIR/.env.example"
mkdir -p "$BUNDLE_DIR/migrations"
cp migrations/*.sql "$BUNDLE_DIR/migrations/"
cp "$SCRIPT_DIR/install.sh" "$BUNDLE_DIR/install.sh"
chmod +x "$BUNDLE_DIR/install.sh"
cp "$SCRIPT_DIR/README.md" "$BUNDLE_DIR/README.md"

echo "== [5/5] Tek dosyaya sikistiriliyor =="
TARBALL="$REPO_ROOT/gokturk-airgapped-bundle.tar.gz"
tar -czf "$TARBALL" -C "$REPO_ROOT" bundle

SIZE="$(du -h "$TARBALL" | cut -f1)"
echo
echo "Bitti: $TARBALL ($SIZE)"
echo "Hedefte: tar xzf gokturk-airgapped-bundle.tar.gz && cd bundle && ./install.sh"
