-- +goose Up
-- seed_targets, OPS-11'in bir tuzagi hangi hedef sunucuya (SSH ile) yerlestirdigini
-- izler. Kimligin SEKLI (username/uid/shell/...) APP-12'nin (internal/seed) isidir
-- ve burada saklanmaz — bu tablo yalnizca "hangi trap, hangi host'a, hangi profille
-- yerlestirildi" bilgisini tutar (bkz. docs/SEEDING.md).
--
-- Bir host icin en fazla bir AKTIF (revoked_at IS NULL) kayit olabilir: partial
-- unique index, "ikinci tetikleme hesabi yeniden olusturmamali" idempotency
-- kabul kriterini DB seviyesinde garanti eder (OPS-11).
CREATE TABLE seed_targets (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trap_id       uuid NOT NULL REFERENCES traps (id),
    host          text NOT NULL,
    ssh_user      text NOT NULL,
    ssh_key_ref   text NOT NULL,
    profile       text NOT NULL,
    seeded_at     timestamptz NOT NULL DEFAULT now(),
    revoked_at    timestamptz
);

CREATE INDEX idx_seed_targets_trap_id ON seed_targets (trap_id);
CREATE UNIQUE INDEX idx_seed_targets_active_host ON seed_targets (host) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS seed_targets;
