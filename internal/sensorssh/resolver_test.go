package sensorssh

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCanaryResolver_Refresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/traps" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{"id":"t1","type":"credential_canary","username":"svc_a","created_at":"2026-07-16T00:00:00Z"},
			{"id":"t2","type":"credential_canary","username":"svc_revoked","created_at":"2026-07-16T00:00:00Z","revoked_at":"2026-07-17T00:00:00Z"}
		]`)
	}))
	defer srv.Close()

	r := NewCanaryResolver(srv.URL, testLogger())
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if id, ok := r.TrapID("svc_a"); !ok || id != "t1" {
		t.Errorf("TrapID(svc_a) = (%q, %v), istenen (t1, true)", id, ok)
	}
	if _, ok := r.TrapID("svc_revoked"); ok {
		t.Error("iptal edilmis tuzak eslemede olmamali")
	}
	if _, ok := r.TrapID("bilinmeyen"); ok {
		t.Error("bilinmeyen kullanici eslemede olmamali")
	}
}

func TestCanaryResolver_Refresh_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bozuk", http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewCanaryResolver(srv.URL, testLogger())
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("500 durumunda hata bekleniyordu")
	}
}

func TestCanaryResolver_Refresh_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "bu json degil")
	}))
	defer srv.Close()

	r := NewCanaryResolver(srv.URL, testLogger())
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("bozuk JSON'da hata bekleniyordu")
	}
}

func TestCanaryResolver_Refresh_KeepsOldOnFailure(t *testing.T) {
	// Once gecerli veriyle doldur, sonra hata veren sunucuya isaret et:
	// eski esleme korunmali.
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"id":"t1","username":"svc_a"}]`)
	}))
	defer ok.Close()

	r := NewCanaryResolver(ok.URL, testLogger())
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("ilk Refresh: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer bad.Close()
	r.apiURL = bad.URL

	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("ikinci Refresh hata dondurmeliydi")
	}
	if id, ok := r.TrapID("svc_a"); !ok || id != "t1" {
		t.Error("hata sonrasi eski esleme korunmaliydi")
	}
}
