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
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq" // "postgres" surucusunu database/sql'e kaydeder

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
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

// InsertTripEvent, trip'i trip_events tablosuna idempotent yazar: ayni
// event_id ikinci kez gelirse hic bir sey yazilmaz ve false doner
// (PLAN APP-6 AC). Raw bos ise '{}' saklanir.
func (s *Store) InsertTripEvent(ctx context.Context, ev trap.TripEvent) (bool, error) {
	const q = `
		INSERT INTO trip_events (event_id, trap_id, sensor, source, observed_at, raw)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6::jsonb, '{}'::jsonb))
		ON CONFLICT (event_id) DO NOTHING`
	// Raw'i string olarak gecir: lib/pq []byte'i bytea (\x..) kodlar ve
	// jsonb cast'i "invalid input syntax for type json" ile patlar.
	var raw any
	if len(ev.Raw) > 0 {
		raw = string(ev.Raw)
	}
	res, err := s.db.ExecContext(ctx, q,
		ev.EventID, ev.TrapID, ev.Sensor, ev.Source, ev.ObservedAt, raw)
	if err != nil {
		return false, fmt.Errorf("store: trip yazilamadi: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: etkilenen satir okunamadi: %w", err)
	}
	return n == 1, nil
}

// ListTripEventsSince, verilen kaynaktan, verilen zamandan itibaren gozlenen
// trip'leri gozlem sirasina gore doner (APP-7 korelasyon penceresi girdisi).
func (s *Store) ListTripEventsSince(ctx context.Context, source string, since time.Time) ([]trap.TripEvent, error) {
	const q = `
		SELECT event_id, trap_id, sensor, source, observed_at, raw
		FROM trip_events
		WHERE source = $1 AND observed_at >= $2
		ORDER BY observed_at ASC`
	rows, err := s.db.QueryContext(ctx, q, source, since)
	if err != nil {
		return nil, fmt.Errorf("store: trip'ler listelenemedi: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []trap.TripEvent
	for rows.Next() {
		var (
			ev  trap.TripEvent
			raw []byte
		)
		if err := rows.Scan(&ev.EventID, &ev.TrapID, &ev.Sensor, &ev.Source, &ev.ObservedAt, &raw); err != nil {
			return nil, fmt.Errorf("store: satir okunamadi: %w", err)
		}
		ev.Raw = raw
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: satirlar okunurken hata: %w", err)
	}
	return events, nil
}

// UpsertAlert, kaynagi icin acik (status=open) bir alarm varsa onu
// gunceller (severity/first_seen/last_seen/trip_count), yoksa yeni bir
// tane olusturur. Boylece ayni kaynaktan ardisik trip'ler tek bir alarm
// satirinda birlesir (PLAN APP-7 AC: "kampanya birlesmesi").
func (s *Store) UpsertAlert(ctx context.Context, a correlate.Alert) (correlate.Alert, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return correlate.Alert{}, fmt.Errorf("store: islem baslatilamadi: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID string
	var firstSeen time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, first_seen FROM alerts
		WHERE source = $1 AND status = $2
		ORDER BY created_at DESC LIMIT 1
		FOR UPDATE`, a.Source, correlate.StatusOpen).Scan(&existingID, &firstSeen)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		const insertQ = `
			INSERT INTO alerts (severity, technique, source, status, first_seen, last_seen, trip_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id::text`
		if scanErr := tx.QueryRowContext(ctx, insertQ,
			a.Severity, a.Technique, a.Source, a.Status, a.FirstSeen, a.LastSeen, a.TripCount,
		).Scan(&a.ID); scanErr != nil {
			return correlate.Alert{}, fmt.Errorf("store: alarm olusturulamadi: %w", scanErr)
		}
	case err != nil:
		return correlate.Alert{}, fmt.Errorf("store: acik alarm aranamadi: %w", err)
	default:
		if a.FirstSeen.After(firstSeen) {
			a.FirstSeen = firstSeen // ilk goruleni asla ileri almayiz
		}
		const updateQ = `
			UPDATE alerts
			SET severity = $1, last_seen = $2, trip_count = $3, first_seen = $4, updated_at = now()
			WHERE id = $5`
		if _, execErr := tx.ExecContext(ctx, updateQ,
			a.Severity, a.LastSeen, a.TripCount, a.FirstSeen, existingID,
		); execErr != nil {
			return correlate.Alert{}, fmt.Errorf("store: alarm guncellenemedi: %w", execErr)
		}
		a.ID = existingID
	}

	if err := tx.Commit(); err != nil {
		return correlate.Alert{}, fmt.Errorf("store: islem onaylanamadi: %w", err)
	}
	return a, nil
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
