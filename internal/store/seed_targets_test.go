package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestStore_CreateSeedTarget_OK(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tp := createTestTrap(t, s, "backup")

	got, err := s.CreateSeedTarget(ctx, SeedTarget{
		TrapID: tp.ID, Host: "10.0.0.50:22", SSHUser: "root",
		SSHKeyRef: "/keys/target.pem", Profile: "debian-service",
	})
	if err != nil {
		t.Fatalf("CreateSeedTarget: %v", err)
	}
	if got.ID == "" {
		t.Error("ID DB tarafindan doldurulmali")
	}
	if got.SeededAt.IsZero() {
		t.Error("SeededAt DB tarafindan doldurulmali")
	}
}

func TestStore_CreateSeedTarget_SecondActiveForSameHostIsRejected(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	host := "10.0.0.51:22"

	trap1 := createTestTrap(t, s, "monitor")
	if _, err := s.CreateSeedTarget(ctx, SeedTarget{
		TrapID: trap1.ID, Host: host, SSHUser: "root", SSHKeyRef: "/k", Profile: "debian-service",
	}); err != nil {
		t.Fatalf("ilk seed: %v", err)
	}

	trap2 := createTestTrap(t, s, "metrics")
	_, err := s.CreateSeedTarget(ctx, SeedTarget{
		TrapID: trap2.ID, Host: host, SSHUser: "root", SSHKeyRef: "/k", Profile: "debian-service",
	})
	if !errors.Is(err, ErrAlreadySeeded) {
		t.Fatalf("beklenen ErrAlreadySeeded, alinan: %v", err)
	}
}

func TestStore_CreateSeedTarget_AfterRevokeSameHostAllowed(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	host := "10.0.0.52:22"

	trap1 := createTestTrap(t, s, "archive")
	first, err := s.CreateSeedTarget(ctx, SeedTarget{
		TrapID: trap1.ID, Host: host, SSHUser: "root", SSHKeyRef: "/k", Profile: "debian-service",
	})
	if err != nil {
		t.Fatalf("ilk seed: %v", err)
	}
	if err := s.RevokeSeedTarget(ctx, first.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	trap2 := createTestTrap(t, s, "collector")
	if _, err := s.CreateSeedTarget(ctx, SeedTarget{
		TrapID: trap2.ID, Host: host, SSHUser: "root", SSHKeyRef: "/k", Profile: "debian-service",
	}); err != nil {
		t.Fatalf("revoke sonrasi ayni host'a yeniden seed edilebilmeli: %v", err)
	}
}

func TestStore_FindActiveSeedTarget(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	host := "10.0.0.53:22"
	tp := createTestTrap(t, s, "sync")

	if _, err := s.CreateSeedTarget(ctx, SeedTarget{
		TrapID: tp.ID, Host: host, SSHUser: "deploy", SSHKeyRef: "/k", Profile: "ops-user",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := s.FindActiveSeedTarget(ctx, host)
	if err != nil {
		t.Fatalf("FindActiveSeedTarget: %v", err)
	}
	if got.SSHUser != "deploy" || got.Profile != "ops-user" {
		t.Errorf("beklenmeyen kayit: %+v", got)
	}
}

func TestStore_FindActiveSeedTarget_NoneReturnsErrNoRows(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	_, err := s.FindActiveSeedTarget(ctx, "10.0.0.99:22")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("beklenen sql.ErrNoRows, alinan: %v", err)
	}
}

func TestStore_RevokeSeedTarget_TwiceFailsSecondTime(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tp := createTestTrap(t, s, "reporter")

	target, err := s.CreateSeedTarget(ctx, SeedTarget{
		TrapID: tp.ID, Host: "10.0.0.54:22", SSHUser: "root", SSHKeyRef: "/k", Profile: "debian-service",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.RevokeSeedTarget(ctx, target.ID); err != nil {
		t.Fatalf("ilk revoke: %v", err)
	}
	if err := s.RevokeSeedTarget(ctx, target.ID); err == nil {
		t.Error("ikinci revoke hata vermeliydi (zaten iptal edilmis)")
	}
}

func TestStore_RevokeTrap_SetsRevokedAt(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	tp := createTestTrap(t, s, "indexer")

	if err := s.RevokeTrap(ctx, tp.ID); err != nil {
		t.Fatalf("RevokeTrap: %v", err)
	}

	list, err := s.ListTraps(ctx)
	if err != nil {
		t.Fatalf("ListTraps: %v", err)
	}
	var revokedAt *time.Time
	found := false
	for _, tr := range list {
		if tr.ID == tp.ID {
			found = true
			revokedAt = tr.RevokedAt
		}
	}
	if !found || revokedAt == nil {
		t.Errorf("tuzak revoked_at ile isaretlenmeli, bulundu=%v revokedAt=%v", found, revokedAt)
	}
}

func TestStore_RevokeTrap_UnknownIDFails(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	if err := s.RevokeTrap(ctx, "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("bilinmeyen id icin hata beklenirdi")
	}
}
