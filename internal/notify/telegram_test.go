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

func TestTelegramChannel_Name(t *testing.T) {
	ch := NewTelegramChannel("t", "c")
	if got := ch.Name(); got != "telegram" {
		t.Errorf("Name() = %q, istenen telegram", got)
	}
}

func TestNewTelegramChannel_EmptyConfigReturnsNil(t *testing.T) {
	if ch := NewTelegramChannel("", "chat"); ch != nil {
		t.Error("bos token ile nil donmeli")
	}
	if ch := NewTelegramChannel("token", ""); ch != nil {
		t.Error("bos chatID ile nil donmeli")
	}
}

func TestTelegramChannel_SendsExpectedRequest(t *testing.T) {
	var gotPath string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewTelegramChannel("test-token", "12345")
	ch.baseURL = srv.URL

	a := correlate.Alert{
		ID: "a1", Severity: correlate.SeverityHigh, Source: "10.0.0.1",
		Status: correlate.StatusOpen, TripCount: 1, FirstSeen: time.Now().UTC(),
	}
	if err := ch.Send(context.Background(), a); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotPath != "/bottest-token/sendMessage" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["chat_id"] != "12345" {
		t.Errorf("chat_id = %q", gotBody["chat_id"])
	}
	if gotBody["text"] == "" {
		t.Error("text bos olmamali")
	}
}

func TestTelegramChannel_NonOKStatusIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	ch := NewTelegramChannel("test-token", "12345")
	ch.baseURL = srv.URL

	if err := ch.Send(context.Background(), correlate.Alert{}); err == nil {
		t.Fatal("403 durum kodu hata donmeliydi")
	}
}
