// Package store, control-api'nin PostgreSQL kalici katmanidir.
// Trap/TripEvent/Alert kayitlari icin ince bir repository saglar; is mantigi
// (uretim, korelasyon) burada degil, ilgili paketlerdedir.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// Store, bir pgx havuzu uzerinden calisir.
type Store struct {
	pool *pgxpool.Pool
}

// New, verilen havuzla bir Store olusturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateTrap, tuzagi traps tablosuna yazar ve DB tarafindan uretilen id/created_at
// ile doldurulmus kaydi dondurur. uuid'ler string olarak okunur (id::text).
func (s *Store) CreateTrap(ctx context.Context, t trap.Trap) (trap.Trap, error) {
	const q = `
		INSERT INTO traps (type, username, secret_hash, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, created_at`
	err := s.pool.QueryRow(ctx, q, t.Type, t.Username, t.SecretHash, t.CreatedBy).
		Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return trap.Trap{}, fmt.Errorf("store: tuzak olusturulamadi: %w", err)
	}
	return t, nil
}

// ListTraps, tuzaklari en yeni once olacak sekilde listeler. secret_hash
// bilincli olarak secilmez — API cevabina asla cikmamali.
func (s *Store) ListTraps(ctx context.Context) ([]trap.Trap, error) {
	const q = `
		SELECT id::text, type, username, created_by, created_at, revoked_at
		FROM traps
		ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store: tuzaklar listelenemedi: %w", err)
	}
	defer rows.Close()

	var traps []trap.Trap
	for rows.Next() {
		var (
			t         trap.Trap
			createdBy *string
		)
		if err := rows.Scan(&t.ID, &t.Type, &t.Username, &createdBy, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, fmt.Errorf("store: satir okunamadi: %w", err)
		}
		if createdBy != nil {
			t.CreatedBy = *createdBy
		}
		traps = append(traps, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: satirlar okunurken hata: %w", err)
	}
	return traps, nil
}
