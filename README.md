<div align="center">

# Göktürk Deception Mesh

**Air-Gapped, Zero-False-Positive Credential Canary Platform (Göktürk Platform P1)**

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![NATS](https://img.shields.io/badge/NATS-JetStream-27AAE1?style=flat-square&logo=natsdotio&logoColor=white)](https://nats.io/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Storage-4169E1?style=flat-square&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Streamlit](https://img.shields.io/badge/SOC%20UI-Streamlit-FF4B4B?style=flat-square&logo=streamlit&logoColor=white)](https://streamlit.io/)
[![Status](https://img.shields.io/badge/MVP-v0.1%20Complete-22c55e?style=flat-square)](PROJECT%20PLAN.md)
[![Org](https://img.shields.io/badge/Org-GokturkFK-0d1117?style=flat-square)](https://github.com/GokturkFK)

*Plant a credential canary (honeytoken), detect attacker usage in ≤5 seconds with zero false positives, correlate campaigns automatically, and fan out to SOC & SIEM—all on a single air-gapped `docker compose up` stack.*

</div>

---

## Demo

![Demo: canary triggered produces High alert within seconds, repeat attempt merges into Critical campaign alert](docs/demo.gif)

*Captured on a live `docker compose up` stack: a credential canary is provisioned, a simulated SSH login triggers a **High** alert in ≤5 seconds, and a second attempt from the same source merges into a single **Critical** campaign alert.*

---

## Overview

**Göktürk Deception Mesh** is Pillar 1 (P1) of the Göktürk Security Platform. Its shared trap contract and deterministic correlation engine live in [`gokturk-core`](https://github.com/GokturkFK/gokturk-core), enabling seamless alert federation alongside [`gokkalkan`](https://github.com/GokturkFK/gokkalkan) (P2) and [`gokzincir`](https://github.com/GokturkFK/gokzincir) (P3).

**Zero-FP by Construction:** The correlation engine intentionally avoids ML anomaly scoring. Every alert stems strictly from a deterministic canary match (`Decode` → known `trap_id`):
- **1 trip** → `High` alert
- **≥2 trips from the same source** → merged into a single `Critical` campaign alert

Full threat model and framework mappings: [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md) · Project plan & Definition of Done: [`PROJECT PLAN.md`](PROJECT%20PLAN.md).

---

## Architecture

A modular monolith (`control-api`: provisioning + ingestion + correlation + alerting + HTTP API in one binary) paired with a lightweight `sensor-ssh` agent over **NATS JetStream**:

```mermaid
flowchart LR
  attacker([Attacker]):::bad -->|SSH login with stolen svc_ canary| target[Target Host]
  target -->|/var/log/auth.log| sensor[sensor-ssh<br/>tail + Decode]
  sensor -->|TripEvent JSON<br/>trip.events.v1| nats[(NATS JetStream<br/>TRIPS stream)]
  nats --> ingest[control-api<br/>ingest consumer]
  ingest -->|idempotent write| pg[(PostgreSQL<br/>traps / trip_events / alerts)]
  ingest --> corr[correlation<br/>Evaluate]
  corr -->|alert| pg
  corr --> fanout[notification fan-out]
  fanout --> siem[siem-sink<br/>syslog CEF]
  fanout --> tg[Telegram]
  fanout --> wh[Webhook]
  operator([SOC Operator]):::ok -->|provision canary| api[control-api HTTP API]
  api --> pg
  dash[dashboard<br/>Streamlit] -->|GET /api/v1/alerts| api
  sensor -.->|GET /api/v1/traps<br/>canary resolution| api
  classDef bad fill:#7f1d1d,color:#fff,stroke:#b91c1c;
  classDef ok fill:#14532d,color:#fff,stroke:#16a34a;
```

### Components

| Component | Responsibility |
| :--- | :--- |
| **`sensor-ssh`** | Tails `auth.log`, runs `ParseAuthLine` → `Decode`; publishes `TripEvent` only for known canaries (`ErrNotATrip` drops all legitimate noise). |
| **`control-api`** | Trap provisioning HTTP API, `trip.events.v1` JetStream consumer, campaign correlation, PostgreSQL persistence, and alert fan-out. |
| **`nats`** | NATS JetStream event bus (`TRIPS` stream, `trip.events.v1` subject). |
| **`postgres`** | Durable storage for `traps`, `trip_events`, and `alerts`. |
| **`dashboard`** | Multi-source Streamlit SOC dashboard consuming `GET /api/v1/alerts` (configurable via `ALERT_SOURCES` to federate P1 + P2 + P3). |
| **`siem-sink`** | Reference Syslog CEF receiver for SIEM integration demos. |

---

## Getting Started

### Prerequisites

- **Go 1.25+**
- **Docker** & **Docker Compose v2+**

### Run the Full Stack

```sh
cp deployments/docker/.env.example deployments/docker/.env   # fill in credentials
make docker-up
```

Brings up all 6 services (`control-api`, `sensor-ssh`, `nats`, `postgres`, `dashboard`, `siem-sink`). See [`deployments/docker/docker-compose.yml`](deployments/docker/docker-compose.yml) for port and environment configuration.

- **Dashboard:** http://localhost:8501 — polls `GET /api/v1/alerts` every 3 seconds.
- `sensor-ssh` mounts `/var/log/auth.log` read-only (Debian/Ubuntu format assumed).

### Multi-Product Dashboard (GKO-4)

The dashboard can show alerts from every Göktürk product that speaks the same `Alert` contract in a **single feed** — no dashboard code changes required, just set `ALERT_SOURCES` in `.env`:

```sh
ALERT_SOURCES=GÖKTÜRK=http://control-api:8080,GÖKKALKAN=http://host.docker.internal:8090,GÖKZİNCİR=http://host.docker.internal:8100
```

URLs must be resolvable **from inside the dashboard container**: use the service name (`control-api`) for services on the same Compose network, or `host.docker.internal` + host port for products running in a separate stack. If left empty, behavior is unchanged and only `CONTROL_API_URL` is used.

### Demo Flow (Definition of Done)

```sh
# 1) Provision a canary — the returned "username" starts with svc_, "secret" is returned once.
#    APP-3: if OPERATOR_TOKEN is set in .env, provisioning requires that token (otherwise 401).
curl -s -XPOST http://localhost:8080/api/v1/traps \
  -H "Authorization: Bearer $OPERATOR_TOKEN" | tee /tmp/canary.json

# 2) Write a fake SSH login line for that username into auth.log (attacker simulation)
U=$(jq -r .username /tmp/canary.json)
logger -p auth.info "sshd[1]: Failed password for $U from 203.0.113.7 port 40012 ssh2"

# 3) Within ≤5 s: High alert on the dashboard + CEF record in siem-sink
docker compose -f deployments/docker/docker-compose.yml logs siem-sink
```

A login with a non-canary user → **no alert at all** (zero-FP proof).

### Automated Trap Seeding

Instead of doing steps 1–2 by hand, `cmd/seeder` generates a believable identity and plants it on a real target server over SSH (account + password + back-dated home directory). Details and full contract: [`docs/SEEDING.md`](docs/SEEDING.md).

### Air-Gapped Installation

The air-gapped claim is not just marketing: `deployments/airgapped/build-bundle.sh` packages every service image into a single archive with `docker save`; the target-side `install.sh` brings the stack up **without any `docker pull` / `build`**. This is genuinely tested in CI — the bundle is built, local images are deleted, and the install is verified to work *only* from the bundle (see the `airgapped-bundle` job in `.github/workflows/ci.yml`). Step by step: [`deployments/airgapped/README.md`](deployments/airgapped/README.md).

### Development & Testing

```sh
make build
make test
make lint
```

### Migrations

```sh
make migrate-up
make migrate-down
```

Schema draft — see the note at the top of [`migrations/00001_init.sql`](migrations/00001_init.sql).

---

## Branch & PR Rules

`main` is protected:
- No direct pushes (including the repo owner) — every change comes via PR.
- All required CI checks must be green before merge: `Build, vet, test`, `golangci-lint`, `Docker build (control-api)`, `Dashboard tests (pytest)`, `Docker build (dashboard)`.
- Approval is not required — if CI is green, the PR author may merge.
- Force-push and branch deletion are disabled on `main`; linear history enforced (**squash** merges only).
- Source branches are deleted automatically after merge.

Branch names are tied to plan task IDs (`APP-*` = Cyber, `OPS-*` = DevOps; see [`PROJECT PLAN.md`](PROJECT%20PLAN.md) §5–6):

```
feature/APP-2-trap-provisioning
fix/OPS-3-ci-gofiles-guard
chore/OPS-1-repo-scaffold
```

---

## Göktürk Security Platform

| Pillar | Repository | Focus |
| :---: | :--- | :--- |
| **Core** | [**`gokturk-core`**](https://github.com/GokturkFK/gokturk-core) | Frozen `trap` wire contract & deterministic `correlate` engine |
| **P1** | **`gokturk-deception-mesh`** *(this repo)* | Credential canaries, SSH sensor, NATS JetStream & unified SOC dashboard |
| **P2** | [**`gokkalkan`**](https://github.com/GokturkFK/gokkalkan) | Agentic AI runtime security — MCP egress proxy & agent honeypot |
| **P3** | [**`gokzincir`**](https://github.com/GokturkFK/gokzincir) | Non-Human Identity (NHI) governance & blast-radius graph |

---

<div align="center">
  <sub>Developed by <a href="https://github.com/GokturkFK">GokturkFK</a> · <a href="CODEOWNERS">CODEOWNERS</a></sub>
</div>
