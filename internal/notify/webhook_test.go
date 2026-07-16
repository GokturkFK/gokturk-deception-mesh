package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
)

func TestWebhookChannel_Name(t *testing.T) {
	ch := NewWebhookChannel("http://example.invalid")
	if got := ch.Name(); got != "webhook" {
		t.Errorf("Name() = %q, istenen webhook", got)
	}
}

func TestNewWebhookChannel_EmptyURLReturnsNil(t *testing.T) {
	if ch := NewWebhookChannel(""); ch != nil {
		t.Error("bos url ile nil donmeli")
	}
}

func TestWebhookChannel_SendsAlertAsJSON(t *testing.T) {
	var got correlate.Alert
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewWebhookChannel(srv.URL)
	want := correlate.Alert{
		ID: "a1", Severity: correlate.SeverityCritical, Source: "10.0.0.1",
		Status: correlate.StatusOpen, TripCount: 2, FirstSeen: time.Now().UTC(),
	}

	if err := ch.Send(context.Background(), want); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if got.ID != want.ID || got.Severity != want.Severity {
		t.Errorf("gonderilen alarm = %+v, alinan = %+v", want, got)
	}
}

func TestWebhookChannel_NonOKStatusIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ch := NewWebhookChannel(srv.URL)
	if err := ch.Send(context.Background(), correlate.Alert{}); err == nil {
		t.Fatal("500 durum kodu hata donmeliydi")
	}
}
