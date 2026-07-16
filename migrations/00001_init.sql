-- +goose Up
-- TASLAK ŞEMA — internal/trap/trap.go (TripEvent) ve internal/correlate/rule.go (Alert)
-- struct'ları Cyber tarafından paylaşılınca birlikte doğrulanacak.
-- Bkz. PROJECT PLAN.md böl. 4 (kontratlar) ve böl. 10 (kontrat kayması riski).
-- Şema değişirse yeni bir migration dosyası ile yapılacak (bu dosya elle düzenlenmeyecek).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE traps (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type          text NOT NULL DEFAULT 'credential_canary',
    username      text NOT NULL UNIQUE,
    secret_hash   text NOT NULL,
    metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by    text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    revoked_at    timestamptz
);

CREATE TABLE trip_events (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id      text NOT NULL UNIQUE,
    trap_id       uuid NOT NULL REFERENCES traps (id),
    sensor        text NOT NULL,
    source        text NOT NULL,
    observed_at   timestamptz NOT NULL,
    raw           jsonb NOT NULL DEFAULT '{}'::jsonb,
    alert_id      uuid,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_trip_events_trap_id ON trip_events (trap_id);
CREATE INDEX idx_trip_events_source ON trip_events (source);

CREATE TABLE alerts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    severity      text NOT NULL CHECK (severity IN ('High', 'Critical')),
    technique     text,
    source        text NOT NULL,
    status        text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'ack', 'closed')),
    first_seen    timestamptz NOT NULL,
    last_seen     timestamptz NOT NULL,
    trip_count    integer NOT NULL DEFAULT 1,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE trip_events
    ADD CONSTRAINT fk_trip_events_alert
    FOREIGN KEY (alert_id) REFERENCES alerts (id);

CREATE INDEX idx_alerts_source ON alerts (source);
CREATE INDEX idx_alerts_status ON alerts (status);

-- +goose Down
ALTER TABLE trip_events DROP CONSTRAINT IF EXISTS fk_trip_events_alert;
DROP TABLE IF EXISTS alerts;
DROP TABLE IF EXISTS trip_events;
DROP TABLE IF EXISTS traps;
