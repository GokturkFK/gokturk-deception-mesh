package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// ErrAlreadySeeded, verilen host icin zaten aktif (revoke edilmemis) bir seed
// kaydi oldugunu belirtir (OPS-11 idempotency AC: "ikinci tetikleme hesabi
// yeniden olusturmamali"). migrations/00002_seed_targets.sql'deki partial
// unique index bunu DB seviyesinde garanti eder; bu hata o kisitin Go
// tarafindaki karsiligidir.
var ErrAlreadySeeded = errors.New("store: bu host icin zaten aktif bir seed kaydi var")

// uniqueViolation, Postgres unique constraint ihlali hata kodudur (23505).
const uniqueViolation = "23505"

// SeedTarget, bir tuzagin SSH ile yerlestirildigi hedefi izler (APP-12'nin
// urettigi kimligin SEKLI degil, "nereye yerlestirildi" bilgisi — bkz.
// docs/SEEDING.md).
type SeedTarget struct {
	ID        string
	TrapID    string
	Host      string
	SSHUser   string
	SSHKeyRef string
	Profile   string
	SeededAt  time.Time
	RevokedAt *time.Time
}

// CreateSeedTarget, bir seed kaydi olusturur. Host icin zaten aktif bir kayit
// varsa ErrAlreadySeeded doner — cagiran (cmd/seeder) bunu "zaten yapilmis,
// tekrar olusturma" olarak yorumlamalidir.
func (s *Store) CreateSeedTarget(ctx context.Context, t SeedTarget) (SeedTarget, error) {
	const q = `
		INSERT INTO seed_targets (trap_id, host, ssh_user, ssh_key_ref, profile)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, seeded_at`
	err := s.db.QueryRowContext(ctx, q, t.TrapID, t.Host, t.SSHUser, t.SSHKeyRef, t.Profile).
		Scan(&t.ID, &t.SeededAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == uniqueViolation {
			return SeedTarget{}, ErrAlreadySeeded
		}
		return SeedTarget{}, fmt.Errorf("store: seed target kaydedilemedi: %w", err)
	}
	return t, nil
}

// FindActiveSeedTarget, verilen host icin aktif (revoke edilmemis) seed
// kaydini doner; yoksa sql.ErrNoRows doner.
func (s *Store) FindActiveSeedTarget(ctx context.Context, host string) (SeedTarget, error) {
	const q = `
		SELECT id::text, trap_id::text, host, ssh_user, ssh_key_ref, profile, seeded_at
		FROM seed_targets
		WHERE host = $1 AND revoked_at IS NULL`
	var t SeedTarget
	err := s.db.QueryRowContext(ctx, q, host).
		Scan(&t.ID, &t.TrapID, &t.Host, &t.SSHUser, &t.SSHKeyRef, &t.Profile, &t.SeededAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SeedTarget{}, err
		}
		return SeedTarget{}, fmt.Errorf("store: aktif seed target aranamadi: %w", err)
	}
	return t, nil
}

// RevokeSeedTarget, verilen id'li seed kaydini iptal edilmis olarak isaretler.
// Hesabin hedeften fiilen kaldirilmasi (userdel) cagiranin (internal/seeder)
// sorumlulugudur; bu yalnizca kaydi gunceller.
func (s *Store) RevokeSeedTarget(ctx context.Context, id string) error {
	const q = `UPDATE seed_targets SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store: seed target iptal edilemedi: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: etkilenen satir okunamadi: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: seed target %s bulunamadi veya zaten iptal edilmis", id)
	}
	return nil
}

// RevokeTrap, tuzagi iptal edilmis olarak isaretler. Sensorun CanaryResolver'i
// (internal/sensorssh/resolver.go) revoked_at dolu tuzaklari atlar; boylece
// seed edilen hesap hedeften kaldirildiktan sonra sensor de onu canary olarak
// tanimayi birakir (OPS-11 revoke akisinin ikinci ayagi).
func (s *Store) RevokeTrap(ctx context.Context, id string) error {
	const q = `UPDATE traps SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store: tuzak iptal edilemedi: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: etkilenen satir okunamadi: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: tuzak %s bulunamadi veya zaten iptal edilmis", id)
	}
	return nil
}
