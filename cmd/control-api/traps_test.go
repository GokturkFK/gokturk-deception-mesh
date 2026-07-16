package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// fakeStore, handler'lari DB olmadan test etmek icin trapStore'u taklit eder.
type fakeStore struct {
	created   []trap.Trap
	createErr error
	listErr   error
}

func (f *fakeStore) CreateTrap(_ context.Context, t trap.Trap) (trap.Trap, error) {
	if f.createErr != nil {
		return trap.Trap{}, f.createErr
	}
	t.ID = "fake-uuid"
	f.created = append(f.created, t)
	return t, nil
}

func (f *fakeStore) ListTraps(_ context.Context) ([]trap.Trap, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.created, nil
}

func newTestAPI(store trapStore) *apiServer {
	return &apiServer{
		provider: trap.NewCredentialCanaryProvider([]byte("0123456789abcdef0123456789abcdef")),
		store:    store,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestHandleCreateTrap_OK(t *testing.T) {
	fs := &fakeStore{}
	api := newTestAPI(fs)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/traps", strings.NewReader(`{"created_by":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	api.handleCreateTrap(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, istenen 201 (govde: %s)", rec.Code, rec.Body.String())
	}
	var resp createTrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("cevap cozulemedi: %v", err)
	}
	if !strings.HasPrefix(resp.Username, "svc_") {
		t.Errorf("username %q svc_ ile baslamali", resp.Username)
	}
	if resp.Secret == "" {
		t.Error("secret cevapta bir kez donmeli")
	}
	if resp.CreatedBy != "alice" {
		t.Errorf("createdBy = %q, istenen alice", resp.CreatedBy)
	}
	if len(fs.created) != 1 {
		t.Errorf("store'a %d kayit yazildi, istenen 1", len(fs.created))
	}
	// Kalici kayitta duz secret bulunmamali (yalnizca hash).
	if fs.created[0].SecretHash == "" || fs.created[0].SecretHash == resp.Secret {
		t.Error("DB'ye yazilan kayit secret'i hash'lenmis olarak tutmali")
	}
}

func TestHandleCreateTrap_DefaultCreatedBy(t *testing.T) {
	fs := &fakeStore{}
	api := newTestAPI(fs)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/traps", nil)
	rec := httptest.NewRecorder()

	api.handleCreateTrap(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, istenen 201", rec.Code)
	}
	if fs.created[0].CreatedBy != "system" {
		t.Errorf("createdBy = %q, bos istekte varsayilan system olmali", fs.created[0].CreatedBy)
	}
}

func TestHandleCreateTrap_BadJSON(t *testing.T) {
	api := newTestAPI(&fakeStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/traps", bytes.NewBufferString(`{bozuk`))
	rec := httptest.NewRecorder()

	api.handleCreateTrap(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, istenen 400", rec.Code)
	}
}

func TestHandleCreateTrap_StoreError(t *testing.T) {
	api := newTestAPI(&fakeStore{createErr: errors.New("db down")})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/traps", nil)
	rec := httptest.NewRecorder()

	api.handleCreateTrap(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, istenen 500", rec.Code)
	}
}

func TestHandleListTraps_Empty(t *testing.T) {
	api := newTestAPI(&fakeStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traps", nil)
	rec := httptest.NewRecorder()

	api.handleListTraps(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, istenen 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("bos liste govdesi = %q, istenen []", got)
	}
}

func TestHandleListTraps_WithItems(t *testing.T) {
	fs := &fakeStore{created: []trap.Trap{
		{ID: "1", Type: trap.TypeCredentialCanary, Username: "svc_aaa", SecretHash: "gizli"},
	}}
	api := newTestAPI(fs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traps", nil)
	rec := httptest.NewRecorder()

	api.handleListTraps(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, istenen 200", rec.Code)
	}
	// secret_hash json:"-" ile isaretli — listede sizmamali.
	if strings.Contains(rec.Body.String(), "gizli") || strings.Contains(rec.Body.String(), "secret_hash") {
		t.Errorf("secret_hash liste cevabina sizdi: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "svc_aaa") {
		t.Errorf("beklenen tuzak listede yok: %s", rec.Body.String())
	}
}
