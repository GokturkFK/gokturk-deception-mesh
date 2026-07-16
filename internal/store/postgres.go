// Package store, control-api'nin PostgreSQL kalici katmanidir.
// Trap/TripEvent/Alert kayitlari icin ince bir repository saglar; is mantigi
// (uretim, korelasyon) burada degil, ilgili paketlerdedir.
//
// Surucu olarak saf-Go lib/pq kullanilir: harici bagimliligi yoktur, boylece
// tedarik zinciri yuzeyi (Trivy/SBOM, OPS-4) minimumdur ve go 1.24 ile uyumludur.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq" // "postgres" surucusunu database/sql'e kaydeder

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// Store, bir database/sql havuzu uzerinden calisir.
type Store struct {
	db *sql.DB
}

// New, verilen *sql.DB ile bir Store olusturur.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateTrap, tuzagi traps tablosuna yazar ve DB tarafindan uretilen id/created_at
// ile doldurulmus kaydi dondurur. uuid'ler string olarak okunur (id::text).
func (s *Store) CreateTrap(ctx context.Context, t trap.Trap) (trap.Trap, error) {
	const q = `
		INSERT INTO traps (type, username, secret_hash, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, created_at`
	err := s.db.QueryRowContext(ctx, q, t.Type, t.Username, t.SecretHash, t.CreatedBy).
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
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store: tuzaklar listelenemedi: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var traps []trap.Trap
	for rows.Next() {
		var (
			t         trap.Trap
			createdBy sql.NullString
			revokedAt sql.NullTime
		)
		if err := rows.Scan(&t.ID, &t.Type, &t.Username, &createdBy, &t.CreatedAt, &revokedAt); err != nil {
			return nil, fmt.Errorf("store: satir okunamadi: %w", err)
		}
		if createdBy.Valid {
			t.CreatedBy = createdBy.String
		}
		if revokedAt.Valid {
			rt := revokedAt.Time
			t.RevokedAt = &rt
		}
		traps = append(traps, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: satirlar okunurken hata: %w", err)
	}
	return traps, nil
}
