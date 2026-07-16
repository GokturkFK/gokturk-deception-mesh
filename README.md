# Göktürk Deception Mesh

MVP v0.1 — bkz. [PROJECT PLAN.md](PROJECT%20PLAN.md) (görev dökümü, mimari, DoD).

Mimari diyagram, tehdit modeli ve demo GIF için bkz. APP-11 (yakında).

## Geliştirme

```sh
cp deployments/docker/.env.example deployments/docker/.env   # değerleri doldur
make build
make test
make lint
```

## Stack'i ayağa kaldırma

```sh
make docker-up
```

Servisler: `control-api`, `sensor-ssh`, `nats`, `postgres`, `dashboard`, `siem-sink`.
Detay için [deployments/docker/docker-compose.yml](deployments/docker/docker-compose.yml).

> `control-api`, `sensor-ssh` ve `dashboard` için Dockerfile'lar henüz eklenmedi (OPS-5).
> O ana kadar `make docker-up` sadece `nats` + `postgres` + `siem-sink`'i healthy şekilde ayağa kaldırır.

## Migration'lar

```sh
make migrate-up
make migrate-down
```

Şema taslak — bkz. [migrations/00001_init.sql](migrations/00001_init.sql) başındaki not.

## Branch & PR kuralları

`main` korumalı:
- Direkt push yok (repo sahibi dahil) — her değişiklik PR ile gelir.
- Merge öncesi `Build, vet, test` + `golangci-lint` CI kontrolleri yeşil olmalı.
- Onay (approval) şart değil — CI yeşilse PR sahibi tek başına merge edebilir.
- Force-push ve branch silme main'de kapalı; linear history zorunlu (merge yalnızca **squash**).
- PR merge olunca kaynak branch otomatik silinir.

Branch adlandırma, plan görev ID'sine bağlı:

```
feature/APP-2-trap-provisioning
fix/OPS-3-ci-gofiles-guard
chore/OPS-1-repo-scaffold
```

`APP-*` = Cyber, `OPS-*` = DevOps (bkz. [PROJECT PLAN.md](PROJECT%20PLAN.md) böl. 5-6).
