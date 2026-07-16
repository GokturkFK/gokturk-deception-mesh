package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
)

// fakeAlertStore, handleListAlerts'i DB olmadan test etmek icin alertLister'i taklit eder.
type fakeAlertStore struct {
	alerts []correlate.Alert
	err    error
}

func (f *fakeAlertStore) ListAlerts(_ context.Context) ([]correlate.Alert, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.alerts, nil
}

func newTestAPIWithAlerts(alerts alertLister) *apiServer {
	api := newTestAPI(&fakeStore{})
	api.alerts = alerts
	return api
}

func TestHandleListAlerts_Empty(t *testing.T) {
	api := newTestAPIWithAlerts(&fakeAlertStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	rec := httptest.NewRecorder()

	api.handleListAlerts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, istenen 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("bos liste govdesi = %q, istenen []", got)
	}
}

func TestHandleListAlerts_WithItems(t *testing.T) {
	fs := &fakeAlertStore{alerts: []correlate.Alert{
		{
			ID: "a1", Severity: correlate.SeverityCritical, Technique: correlate.TechniqueValidAccounts,
			Source: "10.0.0.1", Status: correlate.StatusOpen,
			FirstSeen: time.Now().UTC(), LastSeen: time.Now().UTC(), TripCount: 2,
		},
	}}
	api := newTestAPIWithAlerts(fs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	rec := httptest.NewRecorder()

	api.handleListAlerts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, istenen 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "10.0.0.1") || !strings.Contains(rec.Body.String(), "Critical") {
		t.Errorf("beklenen alarm listede yok: %s", rec.Body.String())
	}
}

func TestHandleListAlerts_StoreError(t *testing.T) {
	api := newTestAPIWithAlerts(&fakeAlertStore{err: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	rec := httptest.NewRecorder()

	api.handleListAlerts(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, istenen 500", rec.Code)
	}
}
